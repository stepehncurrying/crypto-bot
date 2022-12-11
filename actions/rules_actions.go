package actions

import (
	"github.com/slack-go/slack"
)

// HandleSetHigh Sets the high to then tell the user when the crypto value is higher
func HandleSetHigh(splitedText []string, fields []slack.AttachmentField) slack.Attachment {
	return slack.Attachment{}
}

// HandleSetLow Sets the low to then tell the user when the crypto value is lower
func HandleSetLow(splitedText []string, fields []slack.AttachmentField) slack.Attachment {
	return slack.Attachment{}
}
