package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type ResponseCEX struct {
	LastPrice string `json:"lprice"`
	Currency1 string `json:"curr1"`
	Currency2 string `json:"curr2"`
}

func GetCryptoValue(crypto string, currency string) string {
	response, err := http.Get("https://cex.io/api/last_price/" + crypto + "/" + currency)

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	var JsonResponse ResponseCEX
	body, _ := ioutil.ReadAll(response.Body)
	err = json.Unmarshal(body, &JsonResponse)

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
	return JsonResponse.LastPrice
}
