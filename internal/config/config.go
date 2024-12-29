package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// LoadConfig loads the environment variables from the .env file
func LoadConfig() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found. Falling back to system environment variables.")
		os.Create(".env")
	}
}

// GetTelegramBotToken retrieves the Telegram bot token from the environment
func GetTelegramBotToken() string {
	token, exists := os.LookupEnv("TELEGRAM_BOT_TOKEN=")
	if !exists || token == "" {
		log.Println("TELEGRAM_BOT_TOKEN is not set in the environment")
		return ""
	}
	return token
}

// GetTelegramChatID retrieves the Telegram chat ID from the environment
func GetTelegramChatID() string {
	chatID, exists := os.LookupEnv("TELEGRAM_CHAT_ID")
	if !exists || chatID == "" {
		log.Println("TELEGRAM_CHAT_ID is not set in the environment")
		return ""
	}
	return chatID
}
