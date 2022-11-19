package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const NCRYPTOS = 4
const FILE_NAME = "alarms.txt"

type register struct {
	user   string
	date   string
	crypto Cryptos
	price  float64
	rule   Rules
}

type Cryptos int

const (
	BTC Cryptos = iota + 1 // Enum Index = 1
	ETH                    // Enum Index = 2
	SOL                    // Enum Index = 3
	ADA                    // Enum Index = 4
	// ... add Cryptos
)

var (
	cryptosMap = map[string]Cryptos{
		"btc": BTC,
		"eth": ETH,
		"sol": SOL,
		"ada": ADA,
		// ... add Cryptos
	}
)

type Rules int

const (
	upLimit Rules = iota + 1
	downLimit
	// ... add Rules
)

var (
	rulesMap = map[string]Rules{
		"uplimit":   upLimit,
		"downlimit": downLimit,
		// ... add Rules
	}
)

type Response struct {
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
	date := time.Now().String()
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
			crypto := strings.ToUpper(text_splitted[2])
			price := getCryptoValue(crypto, "USD")
			if price != "" {
				attachment.Text = fmt.Sprintf("1 "+crypto+" equals to %s USD", price)
				attachment.Pretext = "As you wanted"
				attachment.Color = "#ff8000"
			} else {
				attachment.Text = fmt.Sprintf("That crypto id doesn't exist")
				attachment.Pretext = "I'm Sorry"
				attachment.Color = "#ff0000"
			}
		}

	case "help":
		// Gives a set of options to the user
		attachment.Pretext = "Here is all I can do!"
		attachment.Text = `Available commands just for you
		- @CryptoBot hello -> Greet me!
		- @CryptoBot cryptoList -> Lists cryptos name to show data or set rules
		- @CryptoBot price any_crypto_name -> Gets the current price of the crypto (if it exists)
		- @CryptoBot setHigh any_crypto_name high_value -> Set a value so I can tell you when the crypto surpasses it
		- @CryptoBot setLow any_crypto_name low_value-> Set a value so I can tell you when the crypto is lower than it
		- @CryptoBot myRules -> Show your active rules
		- @CryptoBot seePerformance any_crypto_name days -> See the performance in the crypto price in the last days
		More to come!`
		attachment.Color = "#0000ff"

	case "cryptolist":
		// Gives a list of crypto names
		attachment.Pretext = "Here goes a list of cryptos you might be interested in"
		attachment.Text = "BTC\nETH\nSOL\nADA\n..."
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
			err := setHigh(userName, date, crypto, highPrice)
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

	case "setlow":
		// Sets the low to then tell the user when the crypto value is lower
		if len(text_splitted) < 4 {
			attachment.Text = fmt.Sprintf("Please try again")
			attachment.Pretext = "Command error"
			attachment.Color = "#ff0000"
		} else {
			crypto := strings.ToUpper(text_splitted[2])
			lowPrice, _ := strconv.ParseFloat(text_splitted[3], 64)
			err := setLow(userName, date, crypto, lowPrice)
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

	case "myRules":
		attachment.Pretext = "To be implemented"
		attachment.Text = "Mock"
		attachment.Color = "#0000ff"

	case "seeperformance":
		attachment.Pretext = "To be implemented"
		attachment.Text = "Mock"
		attachment.Color = "#0000ff"

	default:
		// Send a message to the user
		attachment.Text = fmt.Sprintf("How can I help you %s? Type help after tagging me to know what I can do", user.Name)
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

	err := errors.New("")

	initAttachment := slack.Attachment{}
	initAttachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: time.Now().String(),
		},
	}
	initAttachment.Text = fmt.Sprintf("Hi! I'm on! Type help after tagging me to know what I can do!")
	initAttachment.Pretext = "Howdy!"
	initAttachment.Color = "#4af030"

	_, _, err = api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(initAttachment))

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

	var JsonResponse Response
	body, _ := ioutil.ReadAll(response.Body)
	err = json.Unmarshal(body, &JsonResponse)

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	return JsonResponse.LastPrice
}

func getPricesUSD() (map[string]float64, error) {
	prices := make(map[string]float64)
	var err error
	for i := 1; i <= NCRYPTOS; i++ {
		prices[Cryptos(i).String()], err = strconv.ParseFloat(getCryptoValue(Cryptos(i).String(), "USD"), 64)
		if err != nil {
			return nil, fmt.Errorf("Error getting crypto prices")
		}
	}
	return prices, nil
}

func verifyRules(fileName string, api *slack.Client) error {

	prices, err := getPricesUSD()
	if err != nil {
		return err
	}
	inFile, err := os.Open(FILE_NAME)
	if err != nil {
		return err
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
			if checkRule(*reg, prices) == true {
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

func postMessageRule(reg *register, api *slack.Client) error {
	initAttachment := slack.Attachment{}
	initAttachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: reg.date,
		}, {
			Title: "Initializer",
			Value: reg.user,
		},
	}
	initAttachment.Text = fmt.Sprintf(reg.rule.String()+": "+reg.crypto.String()+" has reached the value %f", reg.price)
	initAttachment.Pretext = "As you requested!"
	initAttachment.Color = "#3aa030"

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(initAttachment))

	if err != nil {
		log.Println("Error", err)
		return err
	}
	return nil
}

func loadData(data string) (*register, error) {
	var e bool
	data_splitted := strings.Split(data, "|")
	crypto, e := parseStringToCrypto(data_splitted[2])
	if e != true {
		return nil, fmt.Errorf("Error parsing the data")
	}
	price, err := strconv.ParseFloat(data_splitted[3], 64)
	if err != nil {
		return nil, fmt.Errorf("Error parsing the data")
	}
	rule, e := parseStringToRule(strings.TrimSuffix(data_splitted[4], "\n"))
	if e != true {
		return nil, fmt.Errorf("Error parsing the data")
	}
	reg := newRegister(data_splitted[0], data_splitted[1], crypto, price, rule)
	return reg, nil
}

func newRegister(user string, date string, crypto Cryptos, price float64, rule Rules) *register {
	var reg register
	reg.user = user
	reg.date = date
	reg.crypto = crypto
	reg.price = price
	reg.rule = rule
	return &reg

}

func checkRule(reg register, prices map[string]float64) bool {

	price, _ := prices[reg.crypto.String()]
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

func setLow(name string, date string, crypto string, price float64) error {
	if isCrypto(crypto) == false {
		return fmt.Errorf("I dont support that Crypto ID")
	}
	if price <= 0 {
		return fmt.Errorf("The price entered is incorrect")
	}
	str := "Active|" + name + "|" + date + "|" + crypto + "|" + fmt.Sprintf("%f", price) + "|" + "downLimit" + "\n"
	b := []byte(str)
	err := saveRule(b)
	if err != nil {
		return fmt.Errorf("Please try again")
	}
	return nil
}

func setHigh(name string, date string, crypto string, price float64) error {
	if isCrypto(crypto) == false {
		return fmt.Errorf("I dont support that Crypto ID")
	}
	if price <= 0 {
		return fmt.Errorf("The price entered is incorrect")
	}
	str := "Active|" + name + "|" + date + "|" + crypto + "|" + fmt.Sprintf("%f", price) + "|" + "upLimit" + "\n"
	b := []byte(str)
	err := saveRule(b)
	if err != nil {
		return fmt.Errorf("Please try again")
	}
	return nil
}

func isCrypto(crypto string) bool {
	for i := 1; i <= NCRYPTOS; i++ {
		if crypto == Cryptos(i).String() {
			return true
		}
	}
	return false
}

func (r Rules) String() string {
	switch r {
	case upLimit:
		return "upLimit"
	case downLimit:
		return "downLImit"
	// ... add Rules
	default:
		return "UNKNOWN"
	}
}

func parseStringToRule(str string) (Rules, bool) {
	r, ok := rulesMap[strings.ToLower(str)]
	return r, ok
}

func (c Cryptos) String() string {
	switch c {
	case BTC:
		return "BTC"
	case ETH:
		return "ETH"
	case SOL:
		return "SOL"
	case ADA:
		return "ADA"
	// ... add Cryptos
	default:
		return "UNKNOWN"
	}
}

func (c Cryptos) cryptoIndex() int {
	return int(c)
}

func saveRule(b []byte) error {
	f, err := os.OpenFile(FILE_NAME, os.O_APPEND|os.O_CREATE, 0644)
	defer f.Close()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	_, err = f.Write(b)
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	return nil
}

func parseStringToCrypto(str string) (Cryptos, bool) {
	c, ok := cryptosMap[strings.ToLower(str)]
	return c, ok
}
