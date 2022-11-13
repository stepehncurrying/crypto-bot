package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"os"
	"time"
)

func main() {
	// Load Env variables from .env file
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Error reading environment variables")
		return
	}

	token := os.Getenv("SLACK_AUTH_TOKEN")
	channel_id := os.Getenv("SLACK_CHANNEL_ID")
	client := slack.New(token, slack.OptionDebug(true))

	attachment := slack.Attachment{
		Pretext: "Super Bot Message",
		Text:    "some text",
		// Color Styles the Text, making it possible to have like Warnings etc.
		Color: "#36a64f",
		// Fields are Optional extra data!
		Fields: []slack.AttachmentField{
			{
				Title: "Date",
				Value: time.Now().String(),
			},
		},
	}

	_, timestamp, err := client.PostMessage(
		channel_id,
		// uncomment the item below to add a extra Header to the message, try it out :)
		//slack.MsgOptionText("New message from bot", false),
		slack.MsgOptionAttachments(attachment),
	)

	if err != nil {
		panic(err)
	}
	fmt.Printf("Message sent at %s", timestamp)
}
