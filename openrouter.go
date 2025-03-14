package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	openRouterAPI        = "https://openrouter.ai/api/v1/chat/completions"
	openRouterCreditsAPI = "https://openrouter.ai/api/v1/credits" // Credits endpoint
)

type CreditsResponse struct {
	Credits float64 `json:"credits"`
	Usage   float64 `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

// GetCredits retrieves the current credits status from OpenRouter
func GetCredits(apiToken string, requestID string) (*CreditsResponse, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("OpenRouter API token is not set")
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", openRouterCreditsAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("HTTP-Referer", "https://t.me/openrouter_bot")
	req.Header.Set("X-Title", "Telegram OpenRouter Bot")
	req.Header.Set("X-Request-ID", requestID)

	startTime := time.Now()
	logDebug("[%s] Sending request to OpenRouter Credits API", requestID)

	// Send request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req = req.WithContext(ctx)
	resp, err := httpClient.Do(req)
	if err != nil {
		logError("[%s] OpenRouter Credits API request failed: %v", requestID, err)
		return nil, fmt.Errorf("request to Credits API failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logError("[%s] Failed to read Credits API response: %v", requestID, err)
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Log API response status and timing
	elapsed := time.Since(startTime)
	logInfo("[%s] OpenRouter Credits API responded with status %d in %v",
		requestID, resp.StatusCode, elapsed)

	// Check status code
	if resp.StatusCode != http.StatusOK {
		logError("[%s] OpenRouter Credits API returned non-OK status: %d, body: %s",
			requestID, resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("API returned error status: %d", resp.StatusCode)
	}

	// Parse response
	var creditsResp CreditsResponse
	if err := json.Unmarshal(bodyBytes, &creditsResp); err != nil {
		logError("[%s] Failed to parse Credits API response: %v, body: %s",
			requestID, err, string(bodyBytes))
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// Check for errors
	if creditsResp.Error != nil {
		logError("[%s] Credits API returned error message: %s", requestID, creditsResp.Error.Message)
		return nil, fmt.Errorf("API error: %s", creditsResp.Error.Message)
	}

	return &creditsResp, nil
}

// FormatCreditsInfo formats the credits information for display
func FormatCreditsInfo(credits *CreditsResponse) string {
	result := "🪙 OpenRouter Credits Information:\n\n"

	// Add remaining credits
	result += fmt.Sprintf("• Remaining credits: %.2f\n", credits.Credits)

	// Add usage if available
	result += fmt.Sprintf("• Usage: %.2f\n", credits.Usage)

	// Add expiration if available
	if credits.ExpiresAt != nil && *credits.ExpiresAt != "" {
		result += fmt.Sprintf("• Expires at: %s\n", *credits.ExpiresAt)
	}

	// Add link to OpenRouter website
	result += "\nView more details at: https://openrouter.ai/account"

	return result
}

// OpenRouterRequest represents a request to the OpenRouter API
type OpenRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message represents a message in the OpenRouter API
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenRouterResponse represents a response from the OpenRouter API
type OpenRouterResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Query the OpenRouter API with context for timeout control
func queryOpenRouterWithContext(ctx context.Context, user User, query string, requestID string) (string, error) {
	modelID := user.Models[user.CurrentModel]
	if modelID == "" {
		return "", fmt.Errorf("model ID not found for %s", user.CurrentModel)
	}

	// Check if context is already done
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("operation cancelled or timed out before API request")
	default:
		// Continue processing
	}

	// Create request
	requestBody := OpenRouterRequest{
		Model: modelID,
		Messages: []Message{
			{
				Role:    "user",
				Content: query,
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "POST", openRouterAPI, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+user.OpenRouterToken)
	req.Header.Set("HTTP-Referer", "https://t.me/openrouter_bot")
	req.Header.Set("X-Title", "Telegram OpenRouter Bot")
	req.Header.Set("X-Request-ID", requestID) // Add request ID to headers for tracing

	startTime := time.Now()
	logDebug("[%s] Sending request to OpenRouter API, model: %s", requestID, modelID)

	// Send request with context and timeout
	resp, err := httpClient.Do(req)
	if err != nil {
		if os.IsTimeout(err) || strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "timeout") {
			logError("[%s] OpenRouter API request timed out after %v", requestID, time.Since(startTime))
			return "", fmt.Errorf("request to AI service timed out (after %v). Please try again", time.Since(startTime))
		}
		logError("[%s] OpenRouter API request failed: %v", requestID, err)
		return "", fmt.Errorf("request to AI service failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response body with timeout
	var bodyBytes []byte
	bodyChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	go func() {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			errChan <- err
			return
		}
		bodyChan <- body
	}()

	// Wait for body read or timeout
	select {
	case <-ctx.Done():
		logError("[%s] Context deadline exceeded while reading response body", requestID)
		return "", fmt.Errorf("timeout while reading response from AI service")
	case err := <-errChan:
		logError("[%s] Failed to read response body: %v", requestID, err)
		return "", fmt.Errorf("failed to read response: %v", err)
	case bodyBytes = <-bodyChan:
		// Successfully read body
	}

	// Log API response status and timing
	elapsed := time.Since(startTime)
	logInfo("[%s] OpenRouter API responded with status %d in %v",
		requestID, resp.StatusCode, elapsed)

	// Check status code
	if resp.StatusCode != http.StatusOK {
		logError("[%s] OpenRouter API returned non-OK status: %d, body: %s",
			requestID, resp.StatusCode, string(bodyBytes))
		return "", fmt.Errorf("API returned error status: %d", resp.StatusCode)
	}

	// Parse response
	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(bodyBytes, &openRouterResp); err != nil {
		logError("[%s] Failed to parse API response: %v, body: %s",
			requestID, err, string(bodyBytes))
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Log successful response parsing
	logDebug("[%s] Successfully parsed API response", requestID)

	// Check for errors
	if openRouterResp.Error != nil {
		logError("[%s] API returned error message: %s", requestID, openRouterResp.Error.Message)
		return "", fmt.Errorf("API error: %s", openRouterResp.Error.Message)
	}

	// Check for empty response
	if len(openRouterResp.Choices) == 0 {
		logError("[%s] API returned empty choices array", requestID)
		return "", fmt.Errorf("no response received from the model")
	}

	responseContent := openRouterResp.Choices[0].Message.Content
	logDebug("[%s] Received valid response from model, length: %d chars",
		requestID, len(responseContent))

	// Clean up any special characters or formatting issues that could cause UTF-8 problems
	responseContent = sanitizeResponse(responseContent, requestID)

	return responseContent, nil
}

// Sanitize response to ensure proper encoding and formatting for Telegram Markdown
func sanitizeResponse(text string, requestID string) string {
	// Ensure text is UTF-8 compliant
	if !utf8.ValidString(text) {
		logWarning("[%s] Response contains invalid UTF-8 sequences, sanitizing", requestID)
		text = strings.Map(func(r rune) rune {
			if r == utf8.RuneError {
				return '�'
			}
			return r
		}, text)
	}

	// Clean up any strange character combinations
	text = strings.ReplaceAll(text, "\r\n", "\n")

	return text
}

// Log a warning message
func logWarning(format string, v ...interface{}) {
	logger.Printf("[WARNING] "+format, v...)
}
