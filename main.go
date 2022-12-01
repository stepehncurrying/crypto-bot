package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const FILE_NAME = "alarms.txt"

type register struct {
	user   string
	date   string
	crypto string
	price  float64
	rule   Rules
}

type Rules int

const (
	highLimit Rules = iota + 1
	lowLimit
)

var (
	rulesMap = map[string]Rules{
		"highlimit": highLimit,
		"lowlimit":  lowLimit,
	}
)

type ResponseCEX struct {
	LastPrice string `json:"lprice"`
	Currency1 string `json:"curr1"`
	Currency2 string `json:"curr2"`
}

func main() {
	// Load Env variables from .env file
	err := godotenv.Load(".env")

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	//channelId := os.Getenv("SLACK_CHANNEL_ID")
	api := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))
	client := socketmode.New(api, socketmode.OptionDebug(false))

	err = initMessage(api)
	// Create a context that can be used to cancel goroutine
	ctx, cancel := context.WithCancel(context.Background())
	// Make this cancel called properly in a real program , graceful shutdown etc

	defer cancel()

	go func(ctx context.Context, api *slack.Client, socketClient *socketmode.Client) {
		// Create a for loop that selects either the context cancellation or the events incomming

		go func() {
			for range time.Tick(time.Second * 10) {
				err := verifyRules(FILE_NAME, api)
				if err != nil {
					log.Fatal(err)
				}
			}
		}()

		for {
			select {
			// inscase context cancel is called exit the goroutine
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:
				// We have a new Events, let's type switch the event
				// Add more use cases here if you want to listen to other events.
				switch event.Type {
				// handle EventAPI events
				case socketmode.EventTypeEventsAPI:
					// The Event sent on the channel is not the same as the EventAPI events so we need to type cast it
					eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type cast the event to the EventsAPIEvent: %v\n", event)
						continue
					}
					// We need to send an Acknowledge to the slack server
					socketClient.Ack(*event.Request)
					// Now we have an Events API event, but this event type can in turn be many types, so we actually need another type switch
					err := handleEventMessage(eventsAPIEvent, api)

					if err != nil {
						log.Fatal(err)
					}
				}
			}
		}
	}(ctx, api, client)

	err = client.Run()
}

func handleEventMessage(event slackevents.EventsAPIEvent, api *slack.Client) error {
	switch event.Type {
	// First we check if this is an CallbackEvent
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		// Yet Another Type switch on the actual Data to see if its an AppMentionEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			// The application has been mentioned since this Event is a Mention event
			var err error
			go func(err *error) {
				*err = handleEventMention(ev, api)
			}(&err)
			if err != nil {
				log.Println("Error", err)
				return err
			}

		}
	default:
		return errors.New("unsupported event type")
	}
	return nil
}

func handleEventMention(event *slackevents.AppMentionEvent, api *slack.Client) error {
	// Grab the user name based on the ID of the one who mentioned the bot
	user, err := api.GetUserInfo(event.User)
	if err != nil {
		return err
	}
	// Check if the user said Hello to the bot
	text := strings.ToLower(event.Text)
	// Create a slice with the arguments
	text_splitted := strings.Split(text, " ")
	dateAux := strings.Split(time.Now().String(), " ")
	date := dateAux[0] + " " + dateAux[1]
	userName := user.Name
	// Create the attachment and assigned based on the message
	attachment := slack.Attachment{}
	// Add Some default context like user who mentioned the bot
	attachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: date,
		}, {
			Title: "Initializer",
			Value: userName,
		},
	}

	switch text_splitted[1] {

	case "hello":
		// Greet the user
		attachment.Text = fmt.Sprintf("Hello %s", user.Name)
		attachment.Pretext = "Greetings"
		attachment.Color = "#4af030"

	case "price":
		if len(text_splitted) < 3 {
			attachment.Text = fmt.Sprintf("You didn't enter any crypto id")
			attachment.Pretext = "I'm Sorry"
			attachment.Color = "#ff0000"
		} else {
			abreviatedCryptoName, found := getAbreviatedCryptoName(text_splitted[2])
			if !found {
				attachment.Text = fmt.Sprintf("I don't support that crypto ID or it doesn't exist (yet)")
				attachment.Pretext = "I'm Sorry"
				attachment.Color = "#ff0000"
			} else {
				price := getCryptoValue(abreviatedCryptoName, "USD")
				attachment.Text = fmt.Sprintf("1 "+abreviatedCryptoName+" equals to %s USD", price)
				attachment.Pretext = "As you wanted"
				attachment.Color = "#ff8000"
			}
		}

	case "help":
		// Gives a set of options to the user
		attachment.Pretext = "Here is all I can do!"
		attachment.Text = `Available commands just for you
		- @CryptoBot hello -> Greet me!
		- @CryptoBot cryptoList -> Lists cryptos name to show data or set rules
		- @CryptoBot price any_crypto_name -> Gets the current price of the crypto (if it exists)
		- @CryptoBot chart any_crypto_name DD-MM-AAAA DD-MM-AAAAA -> Gets the historical market price within a range of dates
		- @CrypyoBot chart any_crypto_name 24h/30d/1y -> Gets the historical market price for last 24 hours, 30 days or 1 year.
		- @CryptoBot setHigh any_crypto_name high_value -> Set a value so I can tell you when the crypto surpasses it
		- @CryptoBot setLow any_crypto_name low_value-> Set a value so I can tell you when the crypto is lower than it
		- @CryptoBot myRules -> Show your active rules
		More to come!`
		attachment.Color = "#0000ff"

	case "cryptolist":
		//time.Sleep(time.Second * 50)
		// Gives a list of crypto names
		attachment.Pretext = "Here goes a list of cryptos you might be interested in"
		attachment.Text = `Feel free to use either the fullname or the abreviation!
		BTC - bitcoin
		ETH - ethereum
		SOL - solana
		ADA - cardano
		DOT - polkadot
		UNI - uniswap
		AAVE - aave`
		attachment.Color = "#0000ff"

	case "sethigh":
		// Sets the high to then tell the user when the crypto value is higher
		if len(text_splitted) < 4 {
			attachment.Text = fmt.Sprintf("Please try again")
			attachment.Pretext = "Command error"
			attachment.Color = "#ff0000"
		} else {
			crypto := strings.ToUpper(text_splitted[2])
			highPrice, _ := strconv.ParseFloat(text_splitted[3], 64)
			if err == nil {
				if highPrice <= 0 {
					attachment.Text = "That´s not a valid value! It must be a possitive number"
					attachment.Pretext = "Try again!"
					attachment.Color = "#ff8000"
				} else {
					err := setBarrierPrice(userName, date, crypto, highPrice, "highLimit")
					if err == nil {
						attachment.Text = "I'll let you know when that happens"
						attachment.Pretext = "Good work!"
						attachment.Color = "#ff8000"
					} else {
						attachment.Text = err.Error()
						attachment.Pretext = "I'm Sorry"
						attachment.Color = "#ff0000"
					}
				}
			} else {
				attachment.Text = err.Error()
				attachment.Pretext = "I'm Sorry"
				attachment.Color = "#ff0000"
			}
		}

	case "setlow":
		// Sets the low to then tell the user when the crypto value is lower
		if len(text_splitted) < 4 {
			attachment.Text = fmt.Sprintf("Please try again")
			attachment.Pretext = "Command error"
			attachment.Color = "#ff0000"
		} else {
			crypto := strings.ToUpper(text_splitted[2])
			lowPrice, err := strconv.ParseFloat(text_splitted[3], 64)
			if err == nil {
				if lowPrice <= 0 {
					attachment.Text = "That´s not a valid value! It must be a possitive number"
					attachment.Pretext = "Try again!"
					attachment.Color = "#ff8000"
				} else {
					err := setBarrierPrice(userName, date, crypto, lowPrice, "lowLimit")
					if err == nil {
						attachment.Text = "I'll let you know when that happens"
						attachment.Pretext = "Good work!"
						attachment.Color = "#ff8000"
					} else {
						attachment.Text = err.Error()
						attachment.Pretext = "I'm Sorry"
						attachment.Color = "#ff0000"
					}
				}
			} else {
				attachment.Text = err.Error()
				attachment.Pretext = "I'm Sorry"
				attachment.Color = "#ff0000"
			}
		}

	case "chart":
		long := len(text_splitted)
		if long < 4 { // arg = 3
			attachment.Text = fmt.Sprintf("Please try again")
			attachment.Pretext = "Command error"
			attachment.Color = "#ff0000"
		} else {
			if long < 5 { // arg = 4
				crypto := text_splitted[2]
				d2 := time.Now()
				r1 := text_splitted[3]
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
					err = fmt.Errorf("That's not a true command!")
				}
				if err == nil {
					attachment.Pretext = "As you wanted"
					attachment.Text = "Here is the historical market price for that data range"
					attachment.Color = "#0000ff"
					attachment.ImageURL = chart
				} else {
					attachment.Text = err.Error()
					attachment.Pretext = "I'm Sorry"
					attachment.Color = "#ff0000"
				}
			} else { // arg = 5
				crypto := text_splitted[2]
				d1, _ := time.Parse(layout, text_splitted[3])
				d2, _ := time.Parse(layout, text_splitted[4])
				chart, err := getChartUrl(crypto, d1, d2)
				if err == nil {
					attachment.Pretext = "As you wanted"
					attachment.Text = "Here is the historical market price for that data range"
					attachment.Color = "#0000ff"
					attachment.ImageURL = chart
				} else {
					attachment.Text = err.Error()
					attachment.Pretext = "I'm Sorry"
					attachment.Color = "#ff0000"
				}
			}
		}

	case "myrules":
		attachment.Pretext = "To be implemented"
		attachment.Text = "Mock"
		attachment.Color = "#0000ff"

	default:
		// Send a message to the user
		attachment.Text = fmt.Sprintf("How can I help you %s? Type 'help' after tagging me to know what I can do", user.Name)
		attachment.Pretext = "That's not a true command!"
		attachment.Color = "#3d3d3d"
	}

	_, _, err = api.PostMessage(event.Channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}

	return nil
}

// Initial message when the bot starts running
func initMessage(api *slack.Client) error {

	dateAux := strings.Split(time.Now().String(), " ")
	date := dateAux[0] + " " + dateAux[1]

	initAttachment := slack.Attachment{}
	initAttachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: date,
		},
	}
	initAttachment.Text = fmt.Sprintf("Hi! I'm on! Type help after tagging me to know what I can do!")
	initAttachment.Pretext = "Howdy!"
	initAttachment.Color = "#4af030"

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(initAttachment))

	if err != nil {
		log.Println("Error", err)
		return err
	}

	return nil
}

func postMessageRule(register *register, api *slack.Client) error {
	initAttachment := slack.Attachment{}
	initAttachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: register.date,
		}, {
			Title: "Initializer",
			Value: register.user,
		},
	}
	initAttachment.Text = fmt.Sprintf(register.rule.String()+": "+register.crypto+" has reached the value %f", register.price)
	initAttachment.Pretext = "As you requested!"
	initAttachment.Color = "#3aa030"

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(initAttachment))

	if err != nil {
		log.Println("Error", err)
		return err
	}
	return nil
}

func getCryptoValue(crypto string, currency string) string {

	response, err := http.Get("https://cex.io/api/last_price/" + crypto + "/" + currency)

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	var JsonResponse ResponseCEX
	body, _ := ioutil.ReadAll(response.Body)
	err = json.Unmarshal(body, &JsonResponse)

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	return JsonResponse.LastPrice
}

func isCrypto(crypto string) bool {
	price := getCryptoValue(crypto, "USD")
	return price != ""
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////// RULES //////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////

func isValueSearched(crypto string, currentCryptoPrices map[string]float64) bool {
	_, valueInMap := currentCryptoPrices[crypto]
	return valueInMap
}

func verifyRules(fileName string, api *slack.Client) error {

	currentCryptoPrices := make(map[string]float64)
	inFile, err := os.Open(FILE_NAME)
	if err != nil {
		return nil
	}
	defer inFile.Close()
	outFile, err := os.OpenFile(FILE_NAME, os.O_RDWR, 0777)
	if err != nil {
		return err
	}
	defer outFile.Close()
	reader := bufio.NewReaderSize(inFile, 10*1024)
	for {
		status, err := reader.ReadString('|')
		if err != nil {
			if err != io.EOF {
				fmt.Println("error:", err)
			}
			break
		}

		switch status {
		case "Active|":
			data, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Println("error:", err)
				}
				break
			}
			reg, err := loadData(data)
			if err != nil {
				return err
			}
			if isPricePastBarrier(*reg, currentCryptoPrices) {
				err := postMessageRule(reg, api)
				if err != nil {
					return err
				}
				outFile.WriteString("Closed|")
			} else {
				outFile.WriteString("Active|")
			}
			outFile.WriteString(data)

		case "Closed|":
			data, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Println("error:", err)
				}
				break
			}
			outFile.WriteString("Closed|" + data)
		}
	}
	return nil
}

func loadData(data string) (*register, error) {
	var success bool
	data_splitted := strings.Split(data, "|")
	crypto := data_splitted[2]
	price, err := strconv.ParseFloat(data_splitted[3], 64)
	if err != nil {
		return nil, fmt.Errorf("Error parsing the price")
	}

	rule, success := parseStringToRule(strings.TrimSuffix(data_splitted[4], "\n"))
	if !success {
		return nil, fmt.Errorf("Error parsing the rule")
	}
	reg := newRegister(data_splitted[0], data_splitted[1], crypto, price, rule)
	return reg, nil
}

func newRegister(user string, date string, crypto string, price float64, rule Rules) *register {
	var reg register
	reg.user = user
	reg.date = date
	reg.crypto = crypto
	reg.price = price
	reg.rule = rule
	return &reg
}

func isPricePastBarrier(reg register, currentCryptoPrices map[string]float64) bool {

	cryptoName := reg.crypto

	price := 0.0

	if !isValueSearched(cryptoName, currentCryptoPrices) {
		price, _ = strconv.ParseFloat(getCryptoValue(cryptoName, "USD"), 64)
		currentCryptoPrices[cryptoName] = price
	} else {
		price = currentCryptoPrices[cryptoName]
	}

	if reg.rule == Rules(1) {
		if reg.price < price {
			return true
		} else {
			return false
		}
	}
	if reg.rule == Rules(2) {
		if price < reg.price {
			return true
		} else {
			return false
		}
	}
	// ... add Rules
	return false
}

func setBarrierPrice(name string, date string, crypto string, price float64, barrierType string) error {

	abreviatedCryptoName, found := getAbreviatedCryptoName(crypto)
	if !found {
		return fmt.Errorf("I don't support that Crypto ID or it doesn't exist (yet)")
	}

	str := ""
	if barrierType == "lowLimit" {
		str = "Active|" + name + "|" + date + "|" + abreviatedCryptoName + "|" + fmt.Sprintf("%f", price) + "|" + "lowLimit" + "\n"
	} else {
		str = "Active|" + name + "|" + date + "|" + abreviatedCryptoName + "|" + fmt.Sprintf("%f", price) + "|" + "highLimit" + "\n"
	}
	b := []byte(str)
	err := saveRule(b)
	if err != nil {
		return fmt.Errorf("Please try again")
	}
	return nil
}

func (r Rules) String() string {
	switch r {
	case highLimit:
		return "highLimit"
	case lowLimit:
		return "lowLimit"
	default:
		return "UNKNOWN"
	}
}

func parseStringToRule(str string) (Rules, bool) {
	r, ok := rulesMap[strings.ToLower(str)]
	return r, ok
}

func saveRule(b []byte) error {
	rulesFile, err := os.OpenFile(FILE_NAME, os.O_APPEND|os.O_CREATE, 0644)
	defer rulesFile.Close()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	_, err = rulesFile.Write(b)
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////// QUICKCHART   //////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////

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

func getChartUrl(crypto string, date1 time.Time, date2 time.Time) (string, error) {

	fullCryptoName, found := getFullCryptoName(crypto)
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

type Response struct {
	Prices        [][]float64 `json:"prices"`
	Market_caps   [][]float64 `json:"market_caps"`
	Total_volumes [][]float64 `json:"total_volumes"`
}

const layout = "02-01-2006"

func HttpGet3(client *http.Client, reqUrl string, headers map[string]string) ([]interface{}, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	respData, err := httpRequest(client, "GET", reqUrl, "", headers)
	if err != nil {
		return nil, err
	}

	var bodyDataMap []interface{}
	err = json.Unmarshal(respData, &bodyDataMap)
	if err != nil {
		log.Println("respData", string(respData))
		return nil, err
	}
	return bodyDataMap, nil
}

func httpRequest(client *http.Client, reqType string, reqUrl string, postData string, requstHeaders map[string]string) ([]byte, error) {
	req, _ := http.NewRequest(reqType, reqUrl, strings.NewReader(postData))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 5.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/31.0.1650.63 Safari/537.36")

	if requstHeaders != nil {
		for k, v := range requstHeaders {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return bodyData, nil
}

const maxData = 250.0

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
		return (splitTime[3] + " " + splitTime[1] + " " + splitTime[6] + " " + splitTime[4])
	}
	return (splitTime[2] + " " + splitTime[1] + " " + splitTime[5] + " " + splitTime[3])
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

/////////////////////////////////////////////////////////////////////////////////////////////////////////////
///////////////////////////////////// Full - abreviated cryptos /////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	abreviatedToFullMap = map[string]string{
		"BTC":  "bitcoin",
		"ETH":  "ethereum",
		"SOL":  "solana",
		"ADA":  "cardano",
		"DOT":  "polkadot",
		"UNI":  "uniswap",
		"AAVE": "aave",
	}
)

var (
	fullToAbreviatedMap = map[string]string{
		"bitcoin":  "BTC",
		"ethereum": "ETH",
		"solana":   "SOL",
		"cardano":  "ADA",
		"polkadot": "DOT",
		"uniswap":  "UNI",
		"aave":     "AAVE",
	}
)

func getFullCryptoName(cryptoName string) (string, bool) {
	cryptoName = strings.ToUpper(cryptoName)
	fullName, found := abreviatedToFullMap[cryptoName]
	if found {
		return fullName, found
	} else {
		cryptoName = strings.ToLower(cryptoName)
		_, found = fullToAbreviatedMap[cryptoName]
		return cryptoName, found
	}
}

func getAbreviatedCryptoName(cryptoName string) (string, bool) {
	cryptoName = strings.ToLower(cryptoName)
	fullName, found := fullToAbreviatedMap[cryptoName]
	if found {
		return fullName, found
	} else {
		cryptoName = strings.ToUpper(cryptoName)
		_, found = abreviatedToFullMap[cryptoName]
		return cryptoName, found
	}
}
