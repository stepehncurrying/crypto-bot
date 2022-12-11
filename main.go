package main

import (
	"bufio"
	"context"
	"crypto-bot/actions"
	"crypto-bot/utils"
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

const FileName = "alarms.txt"

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

func main() {
	// Load Env variables from .env file
	err := godotenv.Load(".env")

	if err != nil {
		fmt.Println("Error getting .env file: ", err)
		return
	}

	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	api := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))
	client := socketmode.New(api, socketmode.OptionDebug(false))

	err = initMessage(api)
	// Create a context that can be used to cancel goroutine
	ctx, cancel := context.WithCancel(context.Background())
	// Make this cancel called properly in a real program , graceful shutdown etc

	defer cancel()

	go func(ctx context.Context, api *slack.Client, socketClient *socketmode.Client) {
		// Every 10 seconds, we check the rules
		go func() {
			for range time.Tick(time.Second * 10) {
				err := verifyRules(FileName, api)
				if err != nil {
					log.Fatal(err)
				}
			}
		}()

		// Create a for loop that selects either the context cancellation or the events incomming
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
		// Yet Another Type switch on the actual Data to see if it's an AppMentionEvent
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
	// Grab the user's name based on the ID of the one who mentioned the bot
	user, err := api.GetUserInfo(event.User)
	if err != nil {
		return err
	}

	mention := strings.ToLower(event.Text)
	// Create a slice with the arguments
	splitedText := strings.Split(mention, " ")

	action := splitedText[1]
	date := utils.GetFormattedActualDate()
	userName := user.Name
	var text, pretext, color string
	var attachment slack.Attachment

	// Add Some default context like user who mentioned the bot
	fields := []slack.AttachmentField{
		{
			Title: "Date",
			Value: date,
		}, {
			Title: "Initializer",
			Value: userName,
		},
	}

	switch action {

	case actions.Hello:
		attachment = actions.HandleHello(user, fields)

	case actions.Sleep:
		attachment = actions.HandleSleep(fields)

	case actions.Price:
		attachment = actions.HandlePrice(splitedText, fields)

	case actions.Help:
		attachment = actions.HandleHelp(fields)

	case actions.CryptoList:
		attachment = actions.HandleCryptoList(fields)

	case actions.SetHigh:
		// Sets the high to then tell the user when the crypto value is higher
		if len(splitedText) < 4 {
			text = fmt.Sprintf("Please try again")
			pretext = "Command error"
			color = "#ff0000"
		} else {
			crypto := strings.ToUpper(splitedText[2])
			highPrice, _ := strconv.ParseFloat(splitedText[3], 64)
			if err == nil {
				if highPrice <= 0 {
					text = "That´s not a valid value! It must be a possitive number"
					pretext = "Try again!"
					color = "#ff8000"
				} else {
					err := setBarrierPrice(userName, date, crypto, highPrice, "highLimit")
					if err == nil {
						text = "I'll let you know when that happens"
						pretext = "Good work!"
						color = "#ff8000"
					} else {
						text = err.Error()
						pretext = "I'm Sorry"
						color = "#ff0000"
					}
				}
			} else {
				text = err.Error()
				pretext = "I'm Sorry"
				color = "#ff0000"
			}
		}

	case actions.SetLow:
		// Sets the low to then tell the user when the crypto value is lower
		if len(splitedText) < 4 {
			text = fmt.Sprintf("Please try again")
			pretext = "Command error"
			color = "#ff0000"
		} else {
			crypto := strings.ToUpper(splitedText[2])
			lowPrice, err := strconv.ParseFloat(splitedText[3], 64)
			if err == nil {
				if lowPrice <= 0 {
					text = "That´s not a valid value! It must be a possitive number"
					pretext = "Try again!"
					color = "#ff8000"
				} else {
					err := setBarrierPrice(userName, date, crypto, lowPrice, "lowLimit")
					if err == nil {
						text = "I'll let you know when that happens"
						pretext = "Good work!"
						color = "#ff8000"
					} else {
						text = err.Error()
						pretext = "I'm Sorry"
						color = "#ff0000"
					}
				}
			} else {
				text = err.Error()
				pretext = "I'm Sorry"
				color = "#ff0000"
			}
		}

	case actions.GetChart:
		attachment = actions.HandleChart(splitedText, fields)

	case actions.MyRules:
		pretext = "To be implemented"
		text = "Mock"
		color = "#0000ff"

	default:
		text = fmt.Sprintf("How can I help you %s? Type 'help' after tagging me to know what I can do", user.Name)
		pretext = "That's not a true command!"
		color = "#3d3d3d"
		attachment = utils.GetAttachment(text, pretext, color, fields, "")
	}

	_, _, err = api.PostMessage(event.Channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}

	return nil
}

// Initial message when the bot starts running
func initMessage(api *slack.Client) error {

	date := utils.GetFormattedActualDate()

	text := fmt.Sprintf("Hi! I'm on! Type help after tagging me to know what I can do!")
	fields := []slack.AttachmentField{
		{
			Title: "Date",
			Value: date,
		},
	}
	attachment := utils.GetAttachment(text, "Howdy!", "#4af030", fields, "")

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(attachment))

	if err != nil {
		log.Println("Error sending message to Slack! ", err)
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

////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////// RULES //////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////

func isValueSearched(crypto string, currentCryptoPrices map[string]float64) bool {
	_, valueInMap := currentCryptoPrices[crypto]
	return valueInMap
}

func verifyRules(fileName string, api *slack.Client) error {

	currentCryptoPrices := make(map[string]float64)
	inFile, err := os.Open(FileName)
	if err != nil {
		return nil
	}
	defer inFile.Close()
	outFile, err := os.OpenFile(FileName, os.O_RDWR, 0777)
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

	abreviatedCryptoName, found := utils.GetAbbreviatedCryptoName(crypto)
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
	rulesFile, err := os.OpenFile(FileName, os.O_APPEND|os.O_CREATE, 0644)
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

func postMessageRule(register *register, api *slack.Client) error {
	fields := []slack.AttachmentField{
		{
			Title: "Date",
			Value: register.date,
		}, {
			Title: "Initializer",
			Value: register.user,
		},
	}

	text := fmt.Sprintf(register.rule.String()+": "+register.crypto+" has reached the value %f", register.price)
	pretext := "As you requested!"
	color := "#3aa030"

	attachment := utils.GetAttachment(text, pretext, color, fields, "")

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(attachment))

	if err != nil {
		log.Println("Error posting message! ", err)
		return err
	}
	return nil
}
