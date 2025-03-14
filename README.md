# OpenRouter Telegram Bot

A Telegram bot that allows users to chat with AI models via OpenRouter API.

## Setup

### Required Environment Variables

This bot requires the following environment variables to be set:

- `TELEGRAM_TOKEN`: Your Telegram bot token obtained from BotFather
- `BOT_PASSWORD`: Password for authorizing users to use the bot

### Optional Environment Variables

- You can also configure other aspects of the bot through environment variables:
    - `LOG_LEVEL`: Set to "debug", "info", or "error" (default is "info")

### Running the Bot

1. Set the required environment variables:

```bash
export TELEGRAM_TOKEN="your_telegram_token_here"
export BOT_PASSWORD="your_secure_password_here"

Run the bot:
go run .
Usage
Start a chat with the bot on Telegram
Enter the password to authorize
Set your OpenRouter API token using /settoken <your_token>
Choose a model with /setmodel <model_name>
Start chatting with the AI!
Available Commands
/help - Show help message
/settoken <token> - Set your OpenRouter API token
/model - Show current AI model
/models - List available AI models
/setmodel <name> - Set current AI model by name
/addmodel <your_name> <openrouter_id> - Add a new model to your list
/removemodel <name> - Remove a model from your list
/debug - Toggle debug logging mode
Security Notes
The bot uses password protection to limit access
Never share your OpenRouter API token or bot password with unauthorized users
User settings and tokens are stored in a local file (bot_config.json)
