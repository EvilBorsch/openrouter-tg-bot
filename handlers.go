package main

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI

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
		return
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
			if user.CurrentModel == modelName {
				user.CurrentModel = ""
			}
			delete(user.Models, modelName)
			updateUser(userID, user, requestID)
			sendMessage(chatID, fmt.Sprintf("Model '%s' removed.", modelName), requestID)
		case "debug":
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
			sendTypingAction(chatID, requestID)
			credits, err := GetCredits(user.OpenRouterToken, requestID)
			if err != nil {
				errMsg := fmt.Sprintf("Error getting credits: %v", err)
				logError("[%s] Failed to get credits: %v", requestID, err)
				sendMessage(chatID, errMsg, requestID)
				return
			}
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
	if user.OpenRouterToken == "" {
		sendMessage(chatID, "Please set your OpenRouter API token first with /settoken <your_token>", requestID)
		return
	}
	if user.CurrentModel == "" || user.Models[user.CurrentModel] == "" {
		sendMessage(chatID, "Please select a model first with /setmodel <model_name>", requestID)
		return
	}

	select {
	case <-ctx.Done():
		logError("[%s] Context expired before API call", requestID)
		return
	default:
		// Continue
	}

	sendTypingAction(chatID, requestID)

	// Send query to OpenRouter
	logInfo("[%s] Sending query to OpenRouter, model: %s, query length: %d chars",
		requestID, user.CurrentModel, len(message.Text))

	response, err := queryOpenRouterWithContext(ctx, user, message.Text, requestID)
	if err != nil {
		errMsg := fmt.Sprintf("Error: %v", err)
		logError("[%s] API request failed: %v", requestID, err)
		sendMessage(chatID, errMsg, requestID)
		return
	}

	logInfo("[%s] Successfully received response from OpenRouter, length: %d chars",
		requestID, len(response))

	cleanedResponse := cleanModelPrefix(response)
	sendMarkdownMessage(chatID, cleanedResponse, requestID)
}

// Send typing action to indicate the bot is processing
func sendTypingAction(chatID int64, requestID string) {
	logDebug("[%s] Sending typing action to chat %d", requestID, chatID)
	_, err := bot.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
	if err != nil {
		logError("[%s] Failed to send typing action: %v", requestID, err)
	}
}

// Clean up any model name prefix in the response
func cleanModelPrefix(text string) string {
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
		if strings.HasPrefix(strings.ToLower(trimmedText), strings.ToLower(prefix)) {
			remainingText := trimmedText[len(prefix):]
			if len(remainingText) > 0 && (remainingText[0] == ' ' || remainingText[0] == '\n') {
				return strings.TrimSpace(remainingText)
			}
		}
	}
	return trimmedText
}

// Send a message in Markdown format (including splitting long messages if needed)
func sendMarkdownMessage(chatID int64, text string, requestID string) {
	logDebug("[%s] Sending Markdown message to chat %d, length: %d chars", requestID, chatID, len(text))

	// Ensure text is UTF-8
	text = ensureUTF8(text)

	// Process the text for Telegram's Markdown
	processedText := convertToTelegramHTML(text)

	// Split if too long
	if len(processedText) > 4000 {
		logInfo("[%s] Message too long (%d chars), splitting into multiple parts", requestID, len(processedText))
		sendMultipartHTMLMessage(chatID, processedText, requestID)
		return
	}

	msg := tgbotapi.NewMessage(chatID, processedText)
	msg.ParseMode = "HTML" // Switch to HTML for more reliable parsing

	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		_, err = bot.Send(msg)
		if err == nil {
			logDebug("[%s] HTML message sent successfully", requestID)
			return
		}
		logError("[%s] Failed to send HTML message (attempt %d/%d): %v", requestID, i+1, maxRetries, err)

		// If HTML parse fails too, try sending as plain text
		if strings.Contains(err.Error(), "can't parse entities") ||
			strings.Contains(err.Error(), "Bad Request") {
			logInfo("[%s] HTML parse failed, sending as plain text", requestID)
			// Strip HTML tags and send as plain text
			plainText := stripHTMLTags(processedText)
			plainMsg := tgbotapi.NewMessage(chatID, plainText)
			_, err = bot.Send(plainMsg)
			if err == nil {
				logDebug("[%s] Plain text message sent successfully", requestID)
				return
			}
			logError("[%s] Failed to send as plain text: %v", requestID, err)
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second) // Exponential backoff
		}
	}

	logError("[%s] Failed to send message after %d attempts", requestID, maxRetries)
	fallbackMsg := tgbotapi.NewMessage(chatID, "I received a response but couldn't display it properly. Please try again.")
	bot.Send(fallbackMsg)
}

// Split and send a large HTML message in multiple parts
func sendMultipartHTMLMessage(chatID int64, text string, requestID string) {
	const maxPartSize = 4000

	var parts []string
	remaining := text

	for len(remaining) > maxPartSize {
		splitIndex := findSplitPoint(remaining, maxPartSize)
		// Ensure HTML tags are properly closed
		part := remaining[:splitIndex]
		part = ensureHTMLTagsClosed(part)
		parts = append(parts, part)
		remaining = remaining[splitIndex:]
	}
	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}

	totalParts := len(parts)
	for i, part := range parts {
		header := ""
		if totalParts > 1 {
			header = fmt.Sprintf("<b>Part %d/%d:</b>\n\n", i+1, totalParts)
		}

		msg := tgbotapi.NewMessage(chatID, header+part)
		msg.ParseMode = "HTML"

		maxRetries := 3
		success := false

		for j := 0; j < maxRetries; j++ {
			_, err := bot.Send(msg)
			if err == nil {
				logDebug("[%s] Part %d/%d sent successfully", requestID, i+1, totalParts)
				success = true
				break
			}
			logError("[%s] Failed to send HTML part %d/%d (attempt %d/%d): %v",
				requestID, i+1, totalParts, j+1, maxRetries, err)

			// If HTML fails, try plain text
			if j == maxRetries-1 {
				plainText := stripHTMLTags(header + part)
				plainMsg := tgbotapi.NewMessage(chatID, plainText)
				_, err = bot.Send(plainMsg)
				if err == nil {
					logDebug("[%s] Part %d/%d sent as plain text", requestID, i+1, totalParts)
					success = true
				} else {
					logError("[%s] Failed to send part %d/%d as plain text: %v",
						requestID, i+1, totalParts, err)
				}
			}

			if j < maxRetries-1 {
				time.Sleep(time.Duration(j+1) * time.Second)
			}
		}

		if !success {
			logError("[%s] Failed to send part %d/%d after all attempts", requestID, i+1, totalParts)
		}
		if i < totalParts-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Convert Markdown to Telegram HTML
func convertToTelegramHTML(text string) string {
	// Replace code blocks
	codeBlockRegex := regexp.MustCompile("```(?:.*?)\n([\\s\\S]*?)```")
	text = codeBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Extract code content
		content := codeBlockRegex.FindStringSubmatch(match)[1]
		// Escape HTML special chars in code
		content = html.EscapeString(content)
		return "<pre>" + content + "</pre>"
	})

	// Replace inline code
	inlineCodeRegex := regexp.MustCompile("`([^`]+)`")
	text = inlineCodeRegex.ReplaceAllString(text, "<code>$1</code>")

	// Replace bold
	text = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(text, "<b>$1</b>")
	text = regexp.MustCompile(`\*(.*?)\*`).ReplaceAllString(text, "<b>$1</b>")

	// Replace italic
	text = regexp.MustCompile(`_(.*?)_`).ReplaceAllString(text, "<i>$1</i>")

	// Handle links - this is simplified
	linkRegex := regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	text = linkRegex.ReplaceAllString(text, "<a href=\"$2\">$1</a>")

	// Convert tables to a simple format that works in Telegram
	text = convertTableToTelegramFormat(text)

	// Properly handle HTML entities
	text = decodeHTMLEntities(text)

	return text
}

// Convert markdown tables to a Telegram-friendly format
func convertTableToTelegramFormat(text string) string {
	// Find all table blocks (assuming they start with | and contain multiple lines)
	tableBlockRegex := regexp.MustCompile(`(?m)^\|(.+)\|$[\r\n]+\|([-:| ]+)\|[\r\n]+((?:\|.+\|$[\r\n]?)+)`)

	return tableBlockRegex.ReplaceAllStringFunc(text, func(tableBlock string) string {
		// Split the table into lines
		lines := strings.Split(tableBlock, "\n")
		if len(lines) < 3 {
			return tableBlock // Not enough lines for a table
		}

		// Process header row
		headerRow := strings.Trim(lines[0], "\r\n")
		headers := extractTableCells(headerRow)

		// Skip the separator line

		// Process data rows
		var resultTable strings.Builder
		resultTable.WriteString("<b>Table:</b>\n\n")

		// Add bold headers
		for i, header := range headers {
			header = strings.TrimSpace(header)
			if i > 0 {
				resultTable.WriteString(" | ")
			}
			resultTable.WriteString("<b>")
			resultTable.WriteString(header)
			resultTable.WriteString("</b>")
		}
		resultTable.WriteString("\n")

		// Add separator
		resultTable.WriteString(strings.Repeat("-", 30))
		resultTable.WriteString("\n")

		// Add data rows
		for i := 2; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "" {
				continue
			}

			cells := extractTableCells(lines[i])
			for j, cell := range cells {
				cell = strings.TrimSpace(cell)
				if j > 0 {
					resultTable.WriteString(" | ")
				}
				resultTable.WriteString(cell)
			}
			resultTable.WriteString("\n")
		}

		return resultTable.String()
	})
}

// Extract cells from a table row
func extractTableCells(row string) []string {
	// Remove the first and last pipe characters
	row = strings.Trim(row, "\r\n")
	if strings.HasPrefix(row, "|") {
		row = row[1:]
	}
	if strings.HasSuffix(row, "|") {
		row = row[:len(row)-1]
	}

	// Split by pipe character
	cells := strings.Split(row, "|")

	// Trim each cell
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}

	return cells
}

// Decode HTML entities that may appear in the text
func decodeHTMLEntities(text string) string {
	// Define common HTML entities and their replacements
	entities := map[string]string{
		"&quot;":  "\"",
		"&#34;":   "\"",
		"&apos;":  "'",
		"&#39;":   "'",
		"&amp;":   "&",
		"&#38;":   "&",
		"&lt;":    "<",
		"&#60;":   "<",
		"&gt;":    ">",
		"&#62;":   ">",
		"&nbsp;":  " ",
		"&#160;":  " ",
		"&ndash;": "–",
		"&#8211;": "–",
		"&mdash;": "—",
		"&#8212;": "—",
	}

	// Replace each entity with its character
	for entity, char := range entities {
		text = strings.ReplaceAll(text, entity, char)
	}

	// Use a more comprehensive approach for numeric entities
	numericEntityRegex := regexp.MustCompile(`&#(\d+);`)
	text = numericEntityRegex.ReplaceAllStringFunc(text, func(match string) string {
		numStr := numericEntityRegex.FindStringSubmatch(match)[1]
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return match
		}
		return string(rune(num))
	})

	return text
}

// Replace escapeNonTagHTML with a more table-friendly version
func escapeNonTagHTML(text string) string {
	// Don't escape content inside these tags
	protectedTags := []string{"pre", "code", "b", "i", "a"}

	// Find all protected tag blocks
	var protectedBlocks []struct {
		start, end int
		content    string
	}

	for _, tag := range protectedTags {
		openTagRegex := regexp.MustCompile(fmt.Sprintf("<%s[^>]*>", tag))
		closeTagRegex := regexp.MustCompile(fmt.Sprintf("</%s>", tag))

		openMatches := openTagRegex.FindAllStringIndex(text, -1)
		closeMatches := closeTagRegex.FindAllStringIndex(text, -1)

		if len(openMatches) > 0 && len(openMatches) == len(closeMatches) {
			for i := 0; i < len(openMatches); i++ {
				start := openMatches[i][0]
				end := closeMatches[i][1]
				if start < end {
					protectedBlocks = append(protectedBlocks, struct {
						start, end int
						content    string
					}{
						start:   start,
						end:     end,
						content: text[start:end],
					})
				}
			}
		}
	}

	// Sort protected blocks by start position
	sort.Slice(protectedBlocks, func(i, j int) bool {
		return protectedBlocks[i].start < protectedBlocks[j].start
	})

	// Build result with escaped text only outside protected blocks
	var result strings.Builder
	lastEnd := 0

	for _, block := range protectedBlocks {
		// Escape text before this block
		if block.start > lastEnd {
			result.WriteString(html.EscapeString(text[lastEnd:block.start]))
		}

		// Add the protected block as-is
		result.WriteString(block.content)
		lastEnd = block.end
	}

	// Escape any remaining text
	if lastEnd < len(text) {
		result.WriteString(html.EscapeString(text[lastEnd:]))
	}

	return result.String()
}

// Strip HTML tags for plain text fallback
func stripHTMLTags(text string) string {
	return regexp.MustCompile("<[^>]*>").ReplaceAllString(text, "")
}

// Ensure HTML tags are properly closed in a fragment
func ensureHTMLTagsClosed(htmlFragment string) string {
	// This is a simplified approach - for production use, consider a proper HTML parser
	openTags := regexp.MustCompile("<([a-z]+)[^>]*>").FindAllStringSubmatch(htmlFragment, -1)
	closedTags := regexp.MustCompile("</([a-z]+)>").FindAllStringSubmatch(htmlFragment, -1)

	// Count open and closed tags
	tagCount := make(map[string]int)
	for _, tag := range openTags {
		tagCount[tag[1]]++
	}
	for _, tag := range closedTags {
		tagCount[tag[1]]--
	}

	// Add closing tags as needed
	for tag, count := range tagCount {
		for i := 0; i < count; i++ {
			htmlFragment += "</" + tag + ">"
		}
	}

	return htmlFragment
}

// Send a simple text message (no special parse mode)
func sendMessage(chatID int64, text string, requestID string) {
	logDebug("[%s] Sending message to chat %d, length: %d chars", requestID, chatID, len(text))
	text = ensureUTF8(text)

	if len(text) > 4000 {
		logInfo("[%s] Message too long (%d chars), splitting into multiple messages", requestID, len(text))
		sendMultipartMessage(chatID, text, requestID)
		return
	}

	msg := tgbotapi.NewMessage(chatID, text)

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		_, err := bot.Send(msg)
		if err == nil {
			logDebug("[%s] Message sent successfully", requestID)
			return
		}
		logError("[%s] Failed to send message (attempt %d/%d): %v",
			requestID, i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	logError("[%s] Failed to send message after %d attempts", requestID, maxRetries)
}

// Send a long message by splitting it into multiple parts (plain text)
func sendMultipartMessage(chatID int64, text string, requestID string) {
	const maxPartSize = 4000

	var parts []string
	remaining := text

	for len(remaining) > maxPartSize {
		splitIndex := findSplitPoint(remaining, maxPartSize)
		parts = append(parts, remaining[:splitIndex])
		remaining = remaining[splitIndex:]
	}
	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}

	totalParts := len(parts)
	for i, part := range parts {
		header := ""
		if totalParts > 1 {
			header = fmt.Sprintf("Part %d/%d:\n\n", i+1, totalParts)
		}

		msg := tgbotapi.NewMessage(chatID, header+part)
		maxRetries := 3
		success := false

		for j := 0; j < maxRetries; j++ {
			_, err := bot.Send(msg)
			if err == nil {
				logDebug("[%s] Part %d/%d sent successfully", requestID, i+1, totalParts)
				success = true
				break
			}
			logError("[%s] Failed to send part %d/%d (attempt %d/%d): %v",
				requestID, i+1, totalParts, j+1, maxRetries, err)
			if j < maxRetries-1 {
				time.Sleep(time.Duration(j+1) * time.Second)
			}
		}
		if !success {
			logError("[%s] Failed to send part %d/%d after all attempts", requestID, i+1, totalParts)
		}
		if i < totalParts-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Find a good point to split a message
func findSplitPoint(text string, maxSize int) int {
	if len(text) <= maxSize {
		return len(text)
	}
	for i := maxSize; i > maxSize/2; i-- {
		if text[i] == '\n' && i > 0 && text[i-1] == '\n' {
			return i + 1
		}
	}
	for i := maxSize; i > maxSize/2; i-- {
		if text[i] == '\n' {
			return i + 1
		}
	}
	for i := maxSize; i > maxSize/2; i-- {
		if i < len(text) && (text[i] == '.' || text[i] == '?' || text[i] == '!') {
			if i+1 < len(text) && text[i+1] == ' ' {
				return i + 2
			}
			return i + 1
		}
	}
	for i := maxSize; i > maxSize/2; i-- {
		if text[i] == ' ' {
			return i + 1
		}
	}
	return maxSize
}

// Ensure text is UTF-8 compliant
func ensureUTF8(text string) string {
	if !utf8.ValidString(text) {
		return strings.Map(func(r rune) rune {
			if r == utf8.RuneError {
				return '�'
			}
			return r
		}, text)
	}
	return text
}
