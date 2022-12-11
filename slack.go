package main

import (
	"fmt"
	"github.com/slack-go/slack"
	"log"
	"os"
)

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

	attachment := getAttachment(text, pretext, color, fields, "")

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(attachment))

	if err != nil {
		log.Println("Error posting message! ", err)
		return err
	}
	return nil
}
