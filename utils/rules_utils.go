package utils

import (
	"fmt"
	"github.com/slack-go/slack"
	"log"
	"os"
	"strconv"
	"strings"
)

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

type Rules int

type register struct {
	user   string
	date   string
	crypto string
	price  float64
	rule   Rules
}

func LoadData(data string) (*register, error) {
	var success bool
	dataSplited := strings.Split(data, "|")
	crypto := dataSplited[2]
	price, err := strconv.ParseFloat(dataSplited[3], 64)
	if err != nil {
		return nil, fmt.Errorf("Error parsing the price")
	}

	rule, success := parseStringToRule(strings.TrimSuffix(dataSplited[4], "\n"))
	if !success {
		return nil, fmt.Errorf("Error parsing the rule")
	}
	reg := newRegister(dataSplited[0], dataSplited[1], crypto, price, rule)
	return reg, nil
}

func IsPricePastBarrier(reg register, currentCryptoPrices map[string]float64) bool {

	cryptoName := reg.crypto

	price := 0.0

	if !isValueSearched(cryptoName, currentCryptoPrices) {
		price, _ = strconv.ParseFloat(GetCryptoValue(cryptoName, "USD"), 64)
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

func PostMessageRule(register *register, api *slack.Client) error {
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

	attachment := GetAttachment(text, pretext, color, fields, "")

	_, _, err := api.PostMessage(os.Getenv("SLACK_CHANNEL_ID"), slack.MsgOptionAttachments(attachment))

	if err != nil {
		log.Println("Error posting message! ", err)
		return err
	}
	return nil
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

func isValueSearched(crypto string, currentCryptoPrices map[string]float64) bool {
	_, valueInMap := currentCryptoPrices[crypto]
	return valueInMap
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
