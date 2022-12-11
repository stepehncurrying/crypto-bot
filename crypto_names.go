package main

var (
	abbreviatedToFullMap = map[string]string{
		"BTC":  "bitcoin",
		"ETH":  "ethereum",
		"SOL":  "solana",
		"ADA":  "cardano",
		"DOT":  "polkadot",
		"UNI":  "uniswap",
		"AAVE": "aave",
	}
)

var (
	fullToAbbreviatedMap = map[string]string{
		"bitcoin":  "BTC",
		"ethereum": "ETH",
		"solana":   "SOL",
		"cardano":  "ADA",
		"polkadot": "DOT",
		"uniswap":  "UNI",
		"aave":     "AAVE",
	}
)
