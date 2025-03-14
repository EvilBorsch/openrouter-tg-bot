# OpenRouter Telegram Bot

A Telegram bot that allows users to chat with AI models via OpenRouter API.
Most code written with help of claude 3.7, so dont really look on overall quality. Function wise bot is pretty good!

## Features

- Chat with any model available on OpenRouter
- Password protection for bot access
- Customizable model list
- Credits balance checking
- Support for reasoning models with step-by-step thinking
- Proper formatting of responses in Telegram

## Installation Options

#### Prerequisites

- Git
- Docker/Go

### Option 1: Docker Installation (Recommended)

````
TELEGRAM_TOKEN=<your_token_from_botfather> BOT_PASSWORD=<password that you and your friend write to access bot> docker-compose up --build -d
````

The bot will automatically restart if it crashes or if the server reboots.

Logs: 
````
docker-compose logs -f
````
Restart the bot: 
````
docker-compose restart
````
Update after code changes:
````
docker-compose down
docker-compose up -d
````

Stop the bot: 

````
docker-compose down
````

### Option 2: Standard Go Installation

#### Setup

1. Set required environment variables:
   ````
   export TELEGRAM_TOKEN="your_telegram_token_here"
   export BOT_PASSWORD="your_secure_password_here"
   ````
3. Run the bot:
   `go run .`

### Usage:

1) Start a chat with the bot on Telegram
2) Enter the password to authorize (set in BOT_PASSWORD environment variable) (only one time)
3) Set your OpenRouter API token using /settoken <your_token> (only one time)
4) Choose a model with /setmodel <model_name>
5) Start chatting with the AI!


### Available Commands
/help - Show help message

/settoken <token> - Set your OpenRouter API token

/model - Show current AI model

/models - List available AI models

/setmodel <name> - Set current AI model by name

/addmodel <name> <id> - Add a new model to your list

/removemodel <name> - Remove a model from your list

/getcredits - Check your OpenRouter credits balance

/debug - Toggle debug logging mode


### Troubleshooting
1) Bot doesn't start: Check that TELEGRAM_TOKEN and BOT_PASSWORD are set correctly
2) Bot doesn't respond: Check the logs for errors
3) Formatting issues: The bot tries to handle various formatting, but some complex markdown might not render correctly


### For my comrads
Contributions are welcome! Please feel free to submit a Pull Request. 

This project is licensed under DO WHAT DO YOU WANT, I DONT CARE license