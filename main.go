package main

import (
	"context"
	"log"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

const (
	defaultTimeout    = 120 * time.Second
	requestTimeout    = 150 * time.Second
	handlerTimeout    = 180 * time.Second
	apiRequestTimeout = 120 * time.Second
)

func main() {
	setupLogger()
	logInfo("Starting bot...")

	// Initialize HTTP client with timeout
	initHTTPClient()

	// Load configuration
	loadConfig()

	var err error
	bot, err = tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		logError("Failed to create Telegram bot: %v", err)
		os.Exit(1)
	}

	logInfo("Bot authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Handle updates
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Generate a request ID for this message
		requestID := uuid.New().String()
		logInfo("[%s] Received message from user %d: %s", requestID, update.Message.From.ID, update.Message.Text)

		// Handle message with timeout
		go func(message *tgbotapi.Message, reqID string) {
			// Create a context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
			defer cancel()

			// Create a done channel to signal completion
			done := make(chan struct{})

			go func() {
				handleMessageWithContext(ctx, message, reqID)
				close(done)
			}()

			select {
			case <-done:
				logInfo("[%s] Message handling completed normally", reqID)
			case <-ctx.Done():
				logError("[%s] Message handling timed out after %v", reqID, handlerTimeout)
				sendMessage(message.Chat.ID, "Sorry, the operation timed out. Please try again.", reqID)
			}
		}(update.Message, requestID)
	}
}

func setupLogger() {
	// Log only to stdout, not to file
	logger = log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("Logger initialized")
}

func logDebug(format string, v ...interface{}) {
	if config.LogLevel == LogLevelDebug {
		logger.Printf("[DEBUG] "+format, v...)
	}
}

func logInfo(format string, v ...interface{}) {
	if config.LogLevel == LogLevelDebug || config.LogLevel == LogLevelInfo {
		logger.Printf("[INFO] "+format, v...)
	}
}

func logError(format string, v ...interface{}) {
	logger.Printf("[ERROR] "+format, v...)
}
