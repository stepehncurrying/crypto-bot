package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
)

func main() {
	// Load Env variables from .env file
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Error reading environment variables")
		return
	}
	fmt.Println(os.Getenv("SLACK_AUTH_TOKEN"))
}
