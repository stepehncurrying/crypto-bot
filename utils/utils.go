package utils

import (
	"github.com/slack-go/slack"
	"strings"
	"time"
)

func GetFormattedActualDate() string {
	timeInfo := strings.Split(time.Now().String(), " ")

	return timeInfo[0] + " " + timeInfo[1]
}

func GetAttachment(text string, pretext string, color string, fields []slack.AttachmentField, image string) slack.Attachment {
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

func GetFullCryptoName(cryptoName string) (string, bool) {
	cryptoName = strings.ToUpper(cryptoName)
	fullName, found := abbreviatedToFullMap[cryptoName]
	if found {
		return fullName, found
	} else {
		cryptoName = strings.ToLower(cryptoName)
		_, found = fullToAbbreviatedMap[cryptoName]
		return cryptoName, found
	}
}

func GetAbbreviatedCryptoName(cryptoName string) (string, bool) {
	cryptoName = strings.ToLower(cryptoName)
	fullName, found := fullToAbbreviatedMap[cryptoName]
	if found {
		return fullName, found
	} else {
		cryptoName = strings.ToUpper(cryptoName)
		_, found = abbreviatedToFullMap[cryptoName]
		return cryptoName, found
	}
}
