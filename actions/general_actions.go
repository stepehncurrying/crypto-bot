package actions

import (
	"crypto-bot/utils"
	"fmt"
	"github.com/slack-go/slack"
	"time"
)

// HandleHello Greet the user
func HandleHello(user *slack.User, fields []slack.AttachmentField) slack.Attachment {
	pretext := "Greetings"
	text := fmt.Sprintf("Hello %s", user.Name)
	color := "#4af030"

	return utils.GetAttachment(text, pretext, color, fields, "")
}

// HandleSleep Sleep for 10 seconds
func HandleSleep(fields []slack.AttachmentField) slack.Attachment {
	time.Sleep(10 * time.Second)
	pretext := "I slept for 10 seconds"
	text := fmt.Sprintf("zzz...")
	color := "#ff33f9"

	return utils.GetAttachment(text, pretext, color, fields, "")
}

// HandleHelp Gives a set of options to the user
func HandleHelp(fields []slack.AttachmentField) slack.Attachment {
	pretext := "Here is all I can do!"
	text := `Available commands just for you
		- @CryptoBot hello -> Greet me!
		- @CryptoBot cryptoList -> Lists cryptos name to show data or set rules
		- @CryptoBot price any_crypto_name -> Gets the current price of the crypto (if it exists)
		- @CryptoBot chart any_crypto_name DD-MM-AAAA DD-MM-AAAAA -> Gets the historical market price within a range of dates
		- @CrypyoBot chart any_crypto_name 24h/30d/1y -> Gets the historical market price for last 24 hours, 30 days or 1 year.
		- @CryptoBot setHigh any_crypto_name high_value -> Set a value so I can tell you when the crypto surpasses it
		- @CryptoBot setLow any_crypto_name low_value-> Set a value so I can tell you when the crypto is lower than it
		- @CryptoBot myRules -> Show your active rules
		More to come!`
	color := "#0000ff"

	return utils.GetAttachment(text, pretext, color, fields, "")
}

// HandleCryptoList Gives a list of the allowed crypto names
func HandleCryptoList(fields []slack.AttachmentField) slack.Attachment {
	pretext := "Here goes a list of cryptos you might be interested in"
	text := `Feel free to use either the fullname or the abreviation!
				BTC - bitcoin
				ETH - ethereum
				SOL - solana
				ADA - cardano
				DOT - polkadot
				UNI - uniswap
				AAVE - aave`
	color := "#0000ff"

	return utils.GetAttachment(text, pretext, color, fields, "")
}
