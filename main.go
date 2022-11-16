package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

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

	// Create the attachment and assigned based on the message
	attachment := slack.Attachment{}
	// Add Some default context like user who mentioned the bot
	attachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: time.Now().String(),
		}, {
			Title: "Initializer",
			Value: user.Name,
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
			price := getCryptoValue(crypto)
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
		- @CryptoBot cryptoList -> Lists many cryptos name to then investigate
		- @CryptoBot price any_crypto_name -> Gets the current price of the crypto (if it exists)
		- @CryptoBot setHigh any_crypto_name high_value -> Set a value so I can tell you when the crypto surpasses it
		- @CryptoBot setLow any_crypto_name low_value-> Set a value so I can tell you when the crypto is lower than it
		- @CryptoBot seePerformance any_crypto_name days -> See the performance in the crypto price in the last days
		More to come!`
		attachment.Color = "#0000ff"

	case "cryptolist":
		// Gives a list of crypto names
		attachment.Pretext = "Here goes a list of cryptos you might be interested in"
		attachment.Text = "BTC\nETH\nBla\n..."
		attachment.Color = "#0000ff"

	case "sethigh":
		// Sets the high to then tell the user when the crypto value is higher
		attachment.Pretext = "To be implemented"
		attachment.Text = "Mock"
		attachment.Color = "#0000ff"

	case "setlow":
		// Sets the high to then tell the user when the crypto value is lower
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

func getCryptoValue(cry string) string {
	response, err := http.Get("https://cex.io/api/last_price/" + cry + "/USD")

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

type Response struct {
	LastPrice string `json:"lprice"`
	Currency1 string `json:"curr1"`
	Currency2 string `json:"curr2"`
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
