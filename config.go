package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Configuration structure
type Config struct {
	TelegramToken string         `json:"telegram_token"`
	Users         map[int64]User `json:"users"`
	AuthorizedIDs map[int64]bool `json:"authorized_ids"` // Track authorized users
	LogLevel      string         `json:"log_level"`      // Log level (debug, info, error)
	// Not storing password in the config file for security
}

// User structure to store per-user settings
type User struct {
	OpenRouterToken string            `json:"openrouter_token"`
	CurrentModel    string            `json:"current_model"`
	Models          map[string]string `json:"models"` // name -> id mapping
}

// Logger levels
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelError = "error"
)

// Global variables
var (
	config      Config
	configMu    sync.Mutex
	httpClient  *http.Client
	logger      *log.Logger
	botPassword string // Store the password separately from the config
)

// Default models to include
var defaultModels = map[string]string{
	"gpt-3.5-turbo":       "openai/gpt-3.5-turbo",
	"gpt-4":               "openai/gpt-4",
	"claude-instant":      "anthropic/claude-instant-v1",
	"claude-2":            "anthropic/claude-2",
	"llama-2-70b":         "meta-llama/llama-2-70b-chat",
	"mistral-7b-instruct": "mistralai/mistral-7b-instruct-v0.1",
}

const (
	configFile = "bot_config.json"
	helpText   = `Available commands:
/help - Show this help message
/settoken <token> - Set your OpenRouter API token
/model - Show current AI model
/models - List available AI models
/setmodel <name> - Set current AI model by name
/addmodel <your_name> <openrouter_id> - Add a new model to your list
/removemodel <name> - Remove a model from your list
/getcredits - Check your OpenRouter credits balance
Just send a message to chat with the current AI model!`
)

// Get the bot password from environment variable
func getBotPassword() string {
	if botPassword == "" {
		botPassword = os.Getenv("BOT_PASSWORD")
	}
	return botPassword
}

// Check if essential environment variables are set
func checkEnvironmentVars() {
	// Check if password is set
	if getBotPassword() == "" {
		logError("BOT_PASSWORD environment variable is not set. Please set it for security.")
		os.Exit(1)
	}

	// You could add other checks here in the future
}

func initHTTPClient() {
	httpClient = &http.Client{
		Timeout: defaultTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// Load configuration from file or create default
func loadConfig() {
	// First check environment variables
	checkEnvironmentVars()

	configMu.Lock()

	config = Config{
		Users:         make(map[int64]User),
		AuthorizedIDs: make(map[int64]bool),
		LogLevel:      LogLevelInfo, // Default log level
	}

	// Try to load existing config
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			logError("Failed to parse config file: %v", err)
		}
	} else {
		logInfo("Config file not found, creating new one")
	}

	// Check if Telegram token is set, if not, get from environment
	if config.TelegramToken == "" {
		config.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
		if config.TelegramToken == "" {
			configMu.Unlock() // Make sure to unlock before fatal
			logError("Telegram token not provided. Set it in config file or TELEGRAM_TOKEN environment variable")
			os.Exit(1)
		}

		// Release the lock before saving to avoid deadlock
		configMu.Unlock()
		saveConfig()
	} else {
		configMu.Unlock()
	}
}

// Save configuration to file
func saveConfig() {
	configMu.Lock()
	defer configMu.Unlock()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logError("Failed to marshal config: %v", err)
		return
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		logError("Failed to write config file: %v", err)
	} else {
		logDebug("Config saved successfully")
	}
}

// Get user from config, initialize if not exists
func getUser(userID int64, requestID string) User {
	configMu.Lock()

	user, exists := config.Users[userID]
	if !exists {
		// Initialize new user with default values
		logInfo("[%s] Creating new user profile for user %d", requestID, userID)
		user = User{
			CurrentModel: "gpt-3.5-turbo",
			Models:       make(map[string]string),
		}
		// Add default models
		for name, id := range defaultModels {
			user.Models[name] = id
		}
		config.Users[userID] = user

		// Release lock before saving
		configMu.Unlock()
		// Save config
		saveConfig()
	} else {
		configMu.Unlock()
		logDebug("[%s] Retrieved existing user profile for user %d", requestID, userID)
	}

	return user
}

// Update user in config
func updateUser(userID int64, user User, requestID string) {
	configMu.Lock()
	config.Users[userID] = user
	configMu.Unlock()

	// Save config after releasing the lock
	saveConfig()
	logDebug("[%s] Updated user profile for user %d", requestID, userID)
}
