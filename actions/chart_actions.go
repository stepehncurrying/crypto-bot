package actions

import (
	"bytes"
	"crypto-bot/utils"
	"encoding/json"
	"fmt"
	"github.com/slack-go/slack"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const layout = "02-01-2006"
const maxData = 250.0

func HandleChart(splitedText []string, fields []slack.AttachmentField) slack.Attachment {
	var text, pretext, color, image string
	args := len(splitedText)
	if args < 4 { // args = 3
		return utils.GetAttachment("Please try again", "Command error", "#ff0000", fields, "")
	} else {
		if args < 5 { // arg = 4
			crypto := splitedText[2]
			d2 := time.Now()
			r1 := splitedText[3]
			var d1 time.Time
			var chart string
			var err error
			switch r1 {
			case "24h":
				d1 = time.Now().AddDate(0, 0, -1)
				chart, err = getChartUrl(crypto, d1, d2)
			case "30d":
				d1 = time.Now().AddDate(0, -1, 0)
				chart, err = getChartUrl(crypto, d1, d2)
			case "1y":
				d1 = time.Now().AddDate(-1, 0, 0)
				chart, err = getChartUrl(crypto, d1, d2)
			default:
				err = fmt.Errorf("that's not a true command")
			}
			if err == nil {
				pretext = "As you wanted"
				text = "Here is the historical market price for that data range"
				color = "#0000ff"
				image = chart
			} else {
				return utils.GetAttachment(err.Error(), "I'm Sorry", "#ff0000", fields, "")
			}
		} else { // arg = 5
			crypto := splitedText[2]
			d1, _ := time.Parse(layout, splitedText[3])
			d2, _ := time.Parse(layout, splitedText[4])
			chart, err := getChartUrl(crypto, d1, d2)
			if err == nil {
				pretext = "As you wanted"
				text = "Here is the historical market price for that data range"
				color = "#0000ff"
				image = chart

			} else {
				return utils.GetAttachment(err.Error(), "I'm Sorry", "#ff0000", fields, "")
			}
		}
	}

	return utils.GetAttachment(text, pretext, color, fields, image)
}

type Chart struct {
	Width             int64   `json:"width"`
	Height            int64   `json:"height"`
	DevicePixelRation float64 `json:"devicePixelRatio"`
	Format            string  `json:"format"`
	BackgroundColor   string  `json:"backgroundColor"`
	Key               string  `json:"key"`
	Version           string  `json:"version,omitempty"`
	Config            string  `json:"chart"`

	Scheme  string        `json:"-"`
	Host    string        `json:"-"`
	Port    int64         `json:"-"`
	Timeout time.Duration `json:"-"`
}

func (quickchart *Chart) validateConfig() bool {
	return len(quickchart.Config) != 0
}

func (quickchart *Chart) GetUrl() (string, error) {

	if !quickchart.validateConfig() {
		return "", fmt.Errorf("invalid config")
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("w=%d", quickchart.Width))
	sb.WriteString(fmt.Sprintf("&h=%d", quickchart.Height))
	sb.WriteString(fmt.Sprintf("&devicePixelRatio=%f", quickchart.DevicePixelRation))
	sb.WriteString(fmt.Sprintf("&f=%s", quickchart.Format))
	sb.WriteString(fmt.Sprintf("&bkg=%s", url.QueryEscape(quickchart.BackgroundColor)))
	sb.WriteString(fmt.Sprintf("&c=%s", url.QueryEscape(quickchart.Config)))

	if len(quickchart.Key) > 0 {
		sb.WriteString(fmt.Sprintf("&key=%s", url.QueryEscape(quickchart.Key)))
	}

	if len(quickchart.Version) > 0 {
		sb.WriteString(fmt.Sprintf("&v=%s", url.QueryEscape(quickchart.Key)))
	}

	return fmt.Sprintf("%s://%s:%d/chart?%s", quickchart.Scheme, quickchart.Host, quickchart.Port, sb.String()), nil

}

type Response struct {
	Prices        [][]float64 `json:"prices"`
	Market_caps   [][]float64 `json:"market_caps"`
	Total_volumes [][]float64 `json:"total_volumes"`
}

func (quickchart *Chart) GetShortUrl() (string, error) {

	if !quickchart.validateConfig() {
		return "", fmt.Errorf("invalid config")
	}

	quickChartURL := fmt.Sprintf("%s://%s:%d/chart/create", quickchart.Scheme, quickchart.Host, quickchart.Port)
	bodyStream, err := quickchart.makePostRequest(quickChartURL)
	if err != nil {
		return "", fmt.Errorf("makePostRequest(%s): %w", quickChartURL, err)
	}

	defer bodyStream.Close()
	body, err := ioutil.ReadAll(bodyStream)
	if err != nil {
		return "", err
	}

	unescapedResponse, err := url.PathUnescape(string(body))
	if err != nil {
		return "", err
	}

	decodedResponse := &getShortURLResponse{}
	err = json.Unmarshal([]byte(unescapedResponse), decodedResponse)
	if err != nil {
		return "", err
	}

	return decodedResponse.URL, nil
}

type getShortURLResponse struct {
	Success bool   `json:"-"`
	URL     string `json:"url"`
}

func (quickchart *Chart) makePostRequest(endpoint string) (io.ReadCloser, error) {
	jsonEncodedPayload, err := json.Marshal(quickchart)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Timeout: quickchart.Timeout,
	}
	resp, err := httpClient.Post(
		endpoint,
		"application/json",
		bytes.NewBuffer(jsonEncodedPayload),
	)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response error: %d - %s", resp.StatusCode, resp.Header.Get("X-quickchart-error"))
	}
	return resp.Body, nil
}

func getChartUrl(crypto string, date1 time.Time, date2 time.Time) (string, error) {

	fullCryptoName, found := utils.GetFullCryptoName(crypto)
	if !found {
		return "", fmt.Errorf("I don't support that Crypto ID or it doesn't exist (yet)")
	}

	tp1u := date1.Unix() + 10800
	fmt.Println(strconv.Itoa(int(tp1u)))
	tp2u := date2.Unix() + 10800
	fmt.Println(strconv.Itoa(int(tp2u)))
	if tp1u >= tp2u {
		return "", fmt.Errorf("Data range is not valid")
	}

	response, err := http.Get("https://api.coingecko.com/api/v3/coins/" + fullCryptoName + "/market_chart/range?vs_currency=usd&from=" + strconv.Itoa(int(tp1u)) + "&to=" + strconv.Itoa(int(tp2u)))

	var JsonResponse Response

	body, _ := ioutil.ReadAll(response.Body)
	err = json.Unmarshal(body, &JsonResponse)

	if err != nil {
		return "", fmt.Errorf("Unexpected error, please try again (Um)")
	}

	prices := JsonResponse.Prices

	cantidadPrecios := len(prices)
	fmt.Println("Cantidad de puntos: ", cantidadPrecios)

	dataJson, _ := buildJSONDataFromData(prices, fullCryptoName)
	quickchart := NewChart()
	quickchart.Config = fmt.Sprintf("{type:'line',%s}", dataJson)

	quickchartURL, err := quickchart.GetShortUrl()
	if err != nil {
		return "", fmt.Errorf("Unexpected error, please try again (Url)")
	}

	return quickchartURL, nil
}

func NewChart() *Chart {
	return &Chart{
		Width:             800,
		Height:            600,
		DevicePixelRation: 1.0,
		Format:            "png",
		BackgroundColor:   "#ffffff",

		Scheme:  "https",
		Host:    "quickchart.io",
		Port:    443,
		Timeout: 10 * time.Second,
	}
}

func buildJSONDataFromData(data [][]float64, crypto string) (string, error) {

	labels := []string{}
	datasetValues := []string{}
	datasetLabel := crypto

	long := len(data)
	sampling := int(math.Ceil(float64(long) / maxData))

	for i := 0; i < long; i += sampling {
		labels = append(labels, getDisplaytime(time.Unix(int64(data[i][0]/1000), 0)))
		datasetValues = append(datasetValues, fmt.Sprintf("%.3f", data[i][1]))
	}

	var sb strings.Builder
	sb.WriteString("data:{labels:[")
	for i, v := range labels {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("'%s'", v))
	}
	sb.WriteString(fmt.Sprintf("],datasets:[{label:'%s',data:[", datasetLabel))
	for i, v := range datasetValues {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("'%s'", v))
	}
	sb.WriteString("]}]}")
	return sb.String(), nil
}

// Set to date format: 1 Jan 2000 00:00:00
func getDisplaytime(unixTime time.Time) string {
	splitTime := strings.Split(unixTime.Format(time.UnixDate), " ")
	if len(splitTime) == 7 { //When day is only 1 number, it fills the blank with another space
		return splitTime[3] + " " + splitTime[1] + " " + splitTime[6] + " " + splitTime[4]
	}
	return splitTime[2] + " " + splitTime[1] + " " + splitTime[5] + " " + splitTime[3]
}
