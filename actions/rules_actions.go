package actions

import (
	"bufio"
	"crypto-bot/utils"
	"fmt"
	"github.com/slack-go/slack"
	"io"
	"os"
	"strconv"
	"strings"
)

// HandleSetLimit Sets the limit to then tell the user when the crypto value is higher
func HandleSetLimit(splitedText []string, userName string, mode string, fields []slack.AttachmentField) slack.Attachment {
	var text, pretext, color string
	date := utils.GetFormattedActualDate()
	if len(splitedText) < 4 {
		text = fmt.Sprintf("Please try again")
		pretext = "Command error"
		color = "#ff0000"
	} else {
		crypto := strings.ToUpper(splitedText[2])
		price, err := strconv.ParseFloat(splitedText[3], 64)
		if err == nil {
			if price <= 0 {
				text = "That´s not a valid value! It must be a possitive number"
				pretext = "Try again!"
				color = "#ff8000"
			} else {
				err := setBarrierPrice(userName, date, crypto, price, mode)
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

	return utils.GetAttachment(text, pretext, color, fields, "")
}

func VerifyRules(fileName string, api *slack.Client) error {

	currentCryptoPrices := make(map[string]float64)
	inFile, err := os.Open(fileName)
	if err != nil {
		return nil
	}
	defer inFile.Close()
	outFile, err := os.OpenFile(fileName, os.O_RDWR, 0777)
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
			reg, err := utils.LoadData(data)
			if err != nil {
				return err
			}
			if utils.IsPricePastBarrier(*reg, currentCryptoPrices) {
				err := utils.PostMessageRule(reg, api)
				if err != nil {
					return err
				}
				outFile.WriteString("Closed|")
			} else {
				outFile.WriteString("Active|")
			}
			outFile.WriteString(data)

		case "Closed|":
			fmt.Print("\nImprimo: ", status, "\n")
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

func setBarrierPrice(name string, date string, crypto string, price float64, barrierType string) error {
	abbreviatedCryptoName, found := utils.GetAbbreviatedCryptoName(crypto)
	if !found {
		return fmt.Errorf("I don't support that Crypto ID or it doesn't exist (yet)")
	}

	str := ""
	if barrierType == "lowLimit" {
		str = "Active|" + name + "|" + date + "|" + abbreviatedCryptoName + "|" + fmt.Sprintf("%f", price) + "|" + "lowLimit" + "\n"
	} else {
		str = "Active|" + name + "|" + date + "|" + abbreviatedCryptoName + "|" + fmt.Sprintf("%f", price) + "|" + "highLimit" + "\n"
	}
	b := []byte(str)
	err := saveRule(b)
	if err != nil {
		return fmt.Errorf("Please try again")
	}
	return nil
}

func saveRule(b []byte) error {
	rulesFile, err := os.OpenFile("alarms.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
