package main

import (
	"context"
	"crypto-bot/actions"
	"crypto-bot/utils"
	"errors"
	"fmt"
	"log"
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
		fmt.Println("Error getting .env file: ", err)
		return
	}

	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	api := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))
	client := socketmode.New(api, socketmode.OptionDebug(false))

	err = actions.InitMessage(api)
	// Create a context that can be used to cancel goroutine
	ctx, cancel := context.WithCancel(context.Background())
	// Make this cancel called properly in a real program , graceful shutdown etc

	defer cancel()

	go func(ctx context.Context, api *slack.Client, socketClient *socketmode.Client) {
		// Every 10 seconds, we check the rules
		go func() {
			for range time.Tick(time.Second * 10) {
				err := actions.VerifyRules(os.Getenv("ALARM_FILENAME"), api)
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
		attachment = actions.HandleSetLimit(splitedText, userName, "highLimit", fields)

	case actions.SetLow:
		attachment = actions.HandleSetLimit(splitedText, userName, "lowLimit", fields)

	case actions.GetChart:
		attachment = actions.HandleChart(splitedText, fields)

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
