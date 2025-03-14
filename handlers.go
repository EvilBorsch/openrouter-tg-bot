package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI

// Check if user is authorized, or handle authorization
// In handlers.go, update the isAuthorized function to use the password from environment variable

// Check if user is authorized, or handle authorization
func isAuthorized(userID int64, message *tgbotapi.Message, requestID string) bool {
	configMu.Lock()
	defer configMu.Unlock()

	// Check if already authorized
	if auth, exists := config.AuthorizedIDs[userID]; exists && auth {
		logDebug("[%s] User %d is already authorized", requestID, userID)
		return true
	}

	// Check if this is a password attempt
	if message.Text == getBotPassword() { // Use the function instead of hardcoded constant
		logInfo("[%s] User %d successfully authorized with password", requestID, userID)
		config.AuthorizedIDs[userID] = true
		go saveConfig() // Save authorization status
		// Inform user of successful authorization
		msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Authorization successful! You can now use the bot.")
		_, err := bot.Send(msg)
		if err != nil {
			logError("[%s] Failed to send authorization success message: %v", requestID, err)
		}
		return true
	}

	// Not authorized - send authorization request
	logInfo("[%s] Unauthorized access attempt by user %d", requestID, userID)
	msg := tgbotapi.NewMessage(message.Chat.ID, "⚠️ This bot is password protected. Please enter the password to continue.")
	_, err := bot.Send(msg)
	if err != nil {
		logError("[%s] Failed to send authorization request message: %v", requestID, err)
	}
	return false
}

// Handle incoming messages with context for timeout control
func handleMessageWithContext(ctx context.Context, message *tgbotapi.Message, requestID string) {
	userID := message.From.ID
	chatID := message.Chat.ID

	// Check if context is already done
	select {
	case <-ctx.Done():
		logError("[%s] Context already expired before message handling", requestID)
		return
	default:
		// Continue processing
	}

	// Check authorization first
	if !isAuthorized(userID, message, requestID) {
		return // Exit if not authorized
	}

	user := getUser(userID, requestID)

	// Check if the message is a command
	if message.IsCommand() {
		cmd := message.Command()
		args := message.CommandArguments()
		logInfo("[%s] Received command /%s from user %d", requestID, cmd, userID)

		switch cmd {
		case "start", "help":
			sendMessage(chatID, helpText, requestID)
		case "settoken":
			if args == "" {
				sendMessage(chatID, "Please provide your OpenRouter API token. Usage: /settoken <your_token>", requestID)
				return
			}
			user.OpenRouterToken = strings.TrimSpace(args)
			updateUser(userID, user, requestID)
			sendMessage(chatID, "OpenRouter API token has been set! You can now chat with AI models.", requestID)
		case "model":
			if user.CurrentModel == "" {
				sendMessage(chatID, "No model selected. Use /setmodel <name> to select a model.", requestID)
			} else {
				modelID := user.Models[user.CurrentModel]
				sendMessage(chatID, fmt.Sprintf("Current model: %s (%s)", user.CurrentModel, modelID), requestID)
			}
		case "models":
			if len(user.Models) == 0 {
				sendMessage(chatID, "No models available. Use /addmodel to add some.", requestID)
				return
			}

			var modelsList string
			for name, id := range user.Models {
				modelsList += fmt.Sprintf("• %s (%s)\n", name, id)
			}
			sendMessage(chatID, fmt.Sprintf("Available models:\n%s\nUse /setmodel <name> to select a model.\n Full models list (for getting ids) can be saw in: https://openrouter.ai/models?order=top-weekly", modelsList), requestID)
		case "setmodel":
			if args == "" {
				sendMessage(chatID, "Please provide a model name. Usage: /setmodel <model_name>", requestID)
				return
			}

			modelName := strings.TrimSpace(args)
			if _, exists := user.Models[modelName]; !exists {
				sendMessage(chatID, fmt.Sprintf("Model '%s' not found. Use /models to see available models.", modelName), requestID)
				return
			}

			user.CurrentModel = modelName
			updateUser(userID, user, requestID)
			sendMessage(chatID, fmt.Sprintf("Model set to: %s (%s)", modelName, user.Models[modelName]), requestID)
		case "addmodel":
			parts := strings.SplitN(args, " ", 2)
			if len(parts) < 2 {
				sendMessage(chatID, "Please provide model name and ID. Usage: /addmodel <your_name> <openrouter_id>", requestID)
				return
			}

			name := strings.TrimSpace(parts[0])
			id := strings.TrimSpace(parts[1])

			if name == "" || id == "" {
				sendMessage(chatID, "Model name and ID cannot be empty.", requestID)
				return
			}

			user.Models[name] = id
			updateUser(userID, user, requestID)
			sendMessage(chatID, fmt.Sprintf("Model added: %s (%s)", name, id), requestID)
		case "removemodel":
			if args == "" {
				sendMessage(chatID, "Please provide a model name. Usage: /removemodel <name>", requestID)
				return
			}

			modelName := strings.TrimSpace(args)
			if _, exists := user.Models[modelName]; !exists {
				sendMessage(chatID, fmt.Sprintf("Model '%s' not found.", modelName), requestID)
				return
			}

			// If removing current model, reset current model
			if user.CurrentModel == modelName {
				user.CurrentModel = ""
			}

			delete(user.Models, modelName)
			updateUser(userID, user, requestID)
			sendMessage(chatID, fmt.Sprintf("Model '%s' removed.", modelName), requestID)
		case "debug":
			// Toggle debug mode for admin troubleshooting
			configMu.Lock()
			if config.LogLevel == LogLevelDebug {
				config.LogLevel = LogLevelInfo
				configMu.Unlock()
				saveConfig()
				sendMessage(chatID, "Debug mode disabled.", requestID)
			} else {
				config.LogLevel = LogLevelDebug
				configMu.Unlock()
				saveConfig()
				sendMessage(chatID, "Debug mode enabled. Check logs for detailed information.", requestID)
			}
		case "getcredits":
			if user.OpenRouterToken == "" {
				sendMessage(chatID, "Please set your OpenRouter API token first with /settoken <your_token>", requestID)
				return
			}

			// Send typing action to indicate processing
			sendTypingAction(chatID, requestID)

			// Get credits information
			credits, err := GetCredits(user.OpenRouterToken, requestID)
			if err != nil {
				errMsg := fmt.Sprintf("Error getting credits: %v", err)
				logError("[%s] Failed to get credits: %v", requestID, err)
				sendMessage(chatID, errMsg, requestID)
				return
			}

			// Format and send credits information
			creditsInfo := FormatCreditsInfo(credits)
			sendMessage(chatID, creditsInfo, requestID)
		default:
			sendMessage(chatID, "Unknown command. Use /help to see available commands.", requestID)
		}
		return
	}

	// Handle regular messages (non-commands)
	if message.Text == "" {
		sendMessage(chatID, "Please send a text message.", requestID)
		return
	}

	// Check if token is set
	if user.OpenRouterToken == "" {
		sendMessage(chatID, "Please set your OpenRouter API token first with /settoken <your_token>", requestID)
		return
	}

	// Check if model is set
	if user.CurrentModel == "" || user.Models[user.CurrentModel] == "" {
		sendMessage(chatID, "Please select a model first with /setmodel <model_name>", requestID)
		return
	}

	// Check context before proceeding
	select {
	case <-ctx.Done():
		logError("[%s] Context expired before API call", requestID)
		return
	default:
		// Continue processing
	}

	// Send typing action
	sendTypingAction(chatID, requestID)

	// Send query to OpenRouter with context for timeout control
	logInfo("[%s] Sending query to OpenRouter, model: %s, query length: %d chars",
		requestID, user.CurrentModel, len(message.Text))

	response, err := queryOpenRouterWithContext(ctx, user, message.Text, requestID)
	if err != nil {
		errMsg := fmt.Sprintf("Error: %v", err)
		logError("[%s] API request failed: %v", requestID, err)
		sendMessage(chatID, errMsg, requestID)
		return
	}

	// Format and send the response
	logInfo("[%s] Successfully received response from OpenRouter, length: %d chars",
		requestID, len(response))

	// Send the response without model prefix
	cleanedResponse := cleanModelPrefix(response)
	sendHTMLMessage(chatID, cleanedResponse, requestID)
}

// Send typing action to indicate the bot is processing
func sendTypingAction(chatID int64, requestID string) {
	logDebug("[%s] Sending typing action to chat %d", requestID, chatID)

	// Use Request instead of Send for chat actions
	_, err := bot.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
	if err != nil {
		logError("[%s] Failed to send typing action: %v", requestID, err)
	}
}

// Clean up any model name prefix in the response
func cleanModelPrefix(text string) string {
	// Common patterns for model prefixes in responses
	prefixPatterns := []string{
		"assistant:", "assistant", "ai:", "ai",
		"bot:", "bot", "chatgpt:", "chatgpt",
		"gpt:", "gpt", "claude:", "claude",
		"qwen:", "qwen", "mistral:", "mistral",
		"llama:", "llama",
	}

	trimmedText := strings.TrimSpace(text)

	for _, prefix := range prefixPatterns {
		prefixWithColon := prefix + ":"
		if strings.HasPrefix(strings.ToLower(trimmedText), strings.ToLower(prefixWithColon)) {
			return strings.TrimSpace(trimmedText[len(prefixWithColon):])
		}

		// Also check without colon
		if strings.HasPrefix(strings.ToLower(trimmedText), strings.ToLower(prefix)) {
			remainingText := trimmedText[len(prefix):]
			if len(remainingText) > 0 && (remainingText[0] == ' ' || remainingText[0] == '\n') {
				return strings.TrimSpace(remainingText)
			}
		}
	}

	return trimmedText
}

func sendHTMLMessage(chatID int64, text string, requestID string) {
	logDebug("[%s] Sending HTML message to chat %d, length: %d chars", requestID, chatID, len(text))

	// Ensure text is UTF-8 compliant
	text = ensureUTF8(text)

	// Replace markdown-style links with HTML links
	text = convertMarkdownToHTML(text)

	// Split messages exceeding Telegram's limit
	if len(text) > 4000 {
		logInfo("[%s] Message too long (%d chars), splitting into multiple messages", requestID, len(text))

		// Split the message and send each part
		messageParts := splitLongMessage(text, 4000, true)
		for i, part := range messageParts {
			logDebug("[%s] Sending part %d/%d, length: %d chars", requestID, i+1, len(messageParts), len(part))

			msg := tgbotapi.NewMessage(chatID, part)
			msg.ParseMode = "HTML" // Use HTML mode which supports proper formatting

			sendMessageWithRetry(msg, requestID, i+1, len(messageParts))
		}
		return
	}

	// For messages within the limit, send normally
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML" // Use HTML mode which supports proper formatting

	sendMessageWithRetry(msg, requestID, 1, 1)
}

// Send a regular message
func sendMessage(chatID int64, text string, requestID string) {
	logDebug("[%s] Sending message to chat %d, length: %d chars", requestID, chatID, len(text))

	// Ensure text is UTF-8 compliant
	text = ensureUTF8(text)

	// Split messages exceeding Telegram's limit
	if len(text) > 4000 {
		logInfo("[%s] Message too long (%d chars), splitting into multiple messages", requestID, len(text))

		// Split the message and send each part
		messageParts := splitLongMessage(text, 4000, false)
		for i, part := range messageParts {
			logDebug("[%s] Sending part %d/%d, length: %d chars", requestID, i+1, len(messageParts), len(part))

			msg := tgbotapi.NewMessage(chatID, part)
			sendMessageWithRetry(msg, requestID, i+1, len(messageParts))
		}
		return
	}

	// For messages within the limit, send normally
	msg := tgbotapi.NewMessage(chatID, text)
	sendMessageWithRetry(msg, requestID, 1, 1)
}

// Helper function to send a message with retry logic
func sendMessageWithRetry(msg tgbotapi.MessageConfig, requestID string, partNum, totalParts int) {
	// Add part number indicator for multi-part messages
	if totalParts > 1 {
		partIndicator := fmt.Sprintf("[%d/%d] ", partNum, totalParts)
		msg.Text = partIndicator + msg.Text
	}

	// Add retry logic for sending messages
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		_, err := bot.Send(msg)
		if err == nil {
			logDebug("[%s] Message part %d/%d sent successfully", requestID, partNum, totalParts)
			return
		}

		logError("[%s] Failed to send message part %d/%d (attempt %d/%d): %v",
			requestID, partNum, totalParts, i+1, maxRetries, err)

		// If HTML parsing fails, try sending as plain text
		if msg.ParseMode == "HTML" && (strings.Contains(err.Error(), "can't parse entities") ||
			strings.Contains(err.Error(), "Bad Request")) {
			logInfo("[%s] HTML parsing failed, sending as plain text", requestID)
			msg.ParseMode = ""
			_, err = bot.Send(msg)
			if err == nil {
				logDebug("[%s] Plain message sent successfully", requestID)
				return
			}
			logError("[%s] Failed to send plain message: %v", requestID, err)
		}

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second) // Exponential backoff
		}
	}

	logError("[%s] Failed to send message part %d/%d after %d attempts", requestID, partNum, totalParts, maxRetries)
}

// Split a long message into multiple parts with proper formatting
func splitLongMessage(text string, maxLength int, preserveHTML bool) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var parts []string
	remainingText := text

	for len(remainingText) > 0 {
		if len(remainingText) <= maxLength {
			// Add the remaining text as the last part
			parts = append(parts, remainingText)
			break
		}

		// Find a good split point to avoid breaking words/formatting
		splitPoint := findSplitPoint(remainingText, maxLength, preserveHTML)

		// Extract the current part
		currentPart := remainingText[:splitPoint]
		parts = append(parts, currentPart)

		// Update the remaining text
		remainingText = remainingText[splitPoint:]

		// Check for HTML tag consistency if needed
		if preserveHTML {
			remainingText = ensureHTMLConsistency(currentPart, remainingText)
		}
	}

	return parts
}

// Find an appropriate point to split the message
func findSplitPoint(text string, maxLength int, preserveHTML bool) int {
	// If text is shorter than max, return its length
	if len(text) <= maxLength {
		return len(text)
	}

	// Try to split at paragraph break first
	if idx := strings.LastIndex(text[:maxLength], "\n\n"); idx > maxLength/2 {
		return idx + 2 // Include the paragraph break
	}

	// Next try to split at line break
	if idx := strings.LastIndex(text[:maxLength], "\n"); idx > maxLength/2 {
		return idx + 1 // Include the line break
	}

	// Try to split at sentence end (period followed by space)
	if idx := strings.LastIndex(text[:maxLength], ". "); idx > maxLength/3 {
		return idx + 2 // Include the period and space
	}

	// Finally, split at word boundary
	if idx := strings.LastIndex(text[:maxLength], " "); idx > 0 {
		return idx + 1 // Include the space
	}

	// If all else fails, just split at the maximum length
	return maxLength
}

// Ensure HTML tags are properly balanced after splitting
func ensureHTMLConsistency(previousPart, nextPart string) string {
	// Check for unclosed tags in the previous part
	openTags := findUnclosedTags(previousPart)

	// If there are unclosed tags, add them to the beginning of the next part
	if len(openTags) > 0 {
		prefix := ""
		for i := len(openTags) - 1; i >= 0; i-- {
			prefix += "<" + openTags[i] + ">"
		}
		return prefix + nextPart
	}

	return nextPart
}

// Find unclosed HTML tags in a string
func findUnclosedTags(html string) []string {
	// Simple regex to find HTML tags
	tagRegex := regexp.MustCompile(`</?([a-z]+)[^>]*>`)
	matches := tagRegex.FindAllStringSubmatch(html, -1)

	// Stack to track open tags
	var openTags []string

	for _, match := range matches {
		tagName := match[1]
		isClosing := strings.HasPrefix(match[0], "</")

		if isClosing {
			// Pop the last open tag if it matches
			if len(openTags) > 0 && openTags[len(openTags)-1] == tagName {
				openTags = openTags[:len(openTags)-1]
			}
		} else {
			// Push the tag onto the stack
			openTags = append(openTags, tagName)
		}
	}

	return openTags
}

// Convert markdown formatting to HTML formatting
func convertMarkdownToHTML(text string) string {
	// First, handle bullet points properly
	bulletPointRegex := regexp.MustCompile(`(?m)^(\s*)[*•]\s(.+)$`)
	text = bulletPointRegex.ReplaceAllString(text, "$1• $2")

	// Handle bold text (convert **text** to <b>text</b>)
	// We'll do this more carefully to ensure proper balancing
	boldParts := strings.Split(text, "**")
	if len(boldParts) > 1 {
		// Rebuild the text with proper HTML bold tags
		newText := boldParts[0]
		for i := 1; i < len(boldParts); i++ {
			if i%2 == 1 {
				// Odd index means this is the content that should be bold
				newText += "<b>" + boldParts[i] + "</b>"
			} else {
				// Even index means this is regular text
				newText += boldParts[i]
			}
		}
		text = newText
	}

	// Replace markdown links with HTML links
	// [text](url) -> <a href="url">text</a>
	markdownLinkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = markdownLinkRegex.ReplaceAllString(text, `<a href="$2">$1</a>`)

	// Handle italic text (single *) but preserve bullet points
	// First, protect bullet points at line beginnings
	text = regexp.MustCompile(`(?m)^(\s*)([*•])\s`).ReplaceAllString(text, "$1BULLETPOINT$2 ")

	// Handle inline italics
	italicParts := strings.Split(text, "*")
	if len(italicParts) > 1 {
		// Rebuild the text with proper HTML italic tags
		newText := italicParts[0]
		for i := 1; i < len(italicParts); i++ {
			if i%2 == 1 {
				// Odd index means this is the content that should be italic
				newText += "<i>" + italicParts[i] + "</i>"
			} else {
				// Even index means this is regular text
				newText += italicParts[i]
			}
		}
		text = newText
	}

	// Restore bullet points
	text = strings.ReplaceAll(text, "BULLETPOINT*", "•")
	text = strings.ReplaceAll(text, "BULLETPOINT•", "•")

	return text
}

// Ensure text is UTF-8 compliant to avoid encoding errors
func ensureUTF8(text string) string {
	if !utf8.ValidString(text) {
		// Replace invalid UTF-8 sequences with Unicode replacement character
		return strings.Map(func(r rune) rune {
			if r == utf8.RuneError {
				return '�'
			}
			return r
		}, text)
	}
	return text
}
