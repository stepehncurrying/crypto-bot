package main

import (
	"github.com/slack-go/slack"
	"strings"
	"time"
)

func getFormattedActualDate() string {
	timeInfo := strings.Split(time.Now().String(), " ")

	return timeInfo[0] + " " + timeInfo[1]
}

func getAttachment(text string, pretext string, color string, fields []slack.AttachmentField, image string) slack.Attachment {
	attachment := slack.Attachment{}
	attachment.Fields = fields
	attachment.Text = text
	attachment.Pretext = pretext
	attachment.Color = color
	if image != "" {
		attachment.ImageURL = image
	}

	return attachment
}
