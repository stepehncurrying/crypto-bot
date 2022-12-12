package actions

import (
	"bytes"
	"crypto-bot/utils"
	"encoding/json"
	"fmt"
	"github.com/slack-go/slack"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type ChartResponse struct {
	Prices       [][]float64 `json:"prices"`
	MarketCaps   [][]float64 `json:"market_caps"`
	TotalVolumes [][]float64 `json:"total_volumes"`
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

type ShortURLResponse struct {
	Success bool   `json:"-"`
	URL     string `json:"url"`
}

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
			d1, _ := time.Parse(utils.Layout, splitedText[3])
			d2, _ := time.Parse(utils.Layout, splitedText[4])
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

	var JsonResponse ChartResponse

	body, _ := ioutil.ReadAll(response.Body)
	err = json.Unmarshal(body, &JsonResponse)

	if err != nil {
		return "", fmt.Errorf("unexpected error, please try again (Um)")
	}

	prices := JsonResponse.Prices

	pricesQuantity := len(prices)
	fmt.Println("Cantidad de puntos: ", pricesQuantity)

	dataJson, _ := utils.BuildJSONDataFromData(prices, fullCryptoName)
	quickChart := NewChart()
	quickChart.Config = fmt.Sprintf("{type:'line',options:{elements:{point:{radius:0}}},%s}", dataJson)

	quickChartURL, err := quickChart.getShortUrl()
	if err != nil {
		return "", fmt.Errorf("unexpected error, please try again (Url)")
	}

	return quickChartURL, nil
}

func NewChart() *Chart {
	return &Chart{
		Width:             1000,
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

func (quickChart *Chart) validateConfig() bool {
	return len(quickChart.Config) != 0
}

func (quickChart *Chart) getShortUrl() (string, error) {

	if !quickChart.validateConfig() {
		return "", fmt.Errorf("invalid config")
	}

	quickChartURL := fmt.Sprintf("%s://%s:%d/chart/create", quickChart.Scheme, quickChart.Host, quickChart.Port)
	bodyStream, err := quickChart.makePostRequest(quickChartURL)
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

	decodedResponse := &ShortURLResponse{}
	err = json.Unmarshal([]byte(unescapedResponse), decodedResponse)
	if err != nil {
		return "", err
	}

	return decodedResponse.URL, nil
}

func (quickChart *Chart) makePostRequest(endpoint string) (io.ReadCloser, error) {
	jsonEncodedPayload, err := json.Marshal(quickChart)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Timeout: quickChart.Timeout,
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
