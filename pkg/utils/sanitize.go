package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// codeFenceRE removes markdown code fences like ```html and ```
var codeFenceRE = regexp.MustCompile("```[a-zA-Z]*\\n?|```")

// SanitizeResponse cleans up model output by removing markdown code fences, inline backticks, and think tags with their content.
// This function serves as the final safety net in our multi-layered approach to handling model outputs.
func SanitizeResponse(s string) string {
	// First remove markdown code fences
	cleaned := codeFenceRE.ReplaceAllString(s, "")
	// Remove inline backticks
	cleaned = strings.ReplaceAll(cleaned, "`", "")

	// Extract thinking content first (for logging or discarding)
	thinking := ExtractThinking(cleaned)
	if thinking != "" {
		// Send thinking to /dev/null (or log it for debugging)
		fmt.Fprintf(io.Discard, "Model thinking: %s\n", thinking)

		// Uncomment the line below if you want to see the thinking in the logs
		// log.Printf("Model thinking from sanitize: %s", thinking)
	}

	// Remove think tags and their content (for DeepSeek models including r-1776)
	// This regex matches <think> tag, any content inside (including newlines), and the closing </think> tag
	cleaned = regexp.MustCompile(`(?i)<think>(?s:.*?)</think>`).ReplaceAllString(cleaned, "")

	// Handle Qwen3 style plain text thinking tags without angle brackets
	cleaned = regexp.MustCompile(`(?i)\bthink\b(?s:.*?)\b/think\b`).ReplaceAllString(cleaned, "")

	// Also try to clean up any JSON-formatted thinking that might be in the response
	// This is a common pattern in models that use JSON for structured outputs
	jsonThinkingRegex := regexp.MustCompile(`(?i)"thinking"\s*:\s*".*?",?`)
	cleaned = jsonThinkingRegex.ReplaceAllString(cleaned, "")

	// Also remove any remaining standalone think tags that might have been split across chunks
	cleaned = regexp.MustCompile(`(?i)(?:\s*<think>(?:\s|\n)*$)`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`(?i)(?:^(?:\s|\n)*</think>\s*)`).ReplaceAllString(cleaned, "")

	// Remove 'html' text at the start of the response when it appears before HTML content
	// This handles cases where the model tries to use Markdown code blocks but doesn't format them correctly
	cleaned = regexp.MustCompile(`^(?i)\s*html\s*\n\s*`).ReplaceAllString(cleaned, ``)
	// Make sure we're not accidentally removing the opening < character
	if strings.HasPrefix(cleaned, "!DOCTYPE") || strings.HasPrefix(cleaned, "html") {
		cleaned = "<" + cleaned
	}

	return cleaned
}

// ExtractThinking attempts to extract thinking/reasoning content from model output
// This can be from <think> tags or from JSON structure
func ExtractThinking(output string) string {
	// First try to extract content from <think> tags
	thinkRegex := regexp.MustCompile(`(?i)<think>((?s:.*?))</think>`)
	matches := thinkRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try to extract content from Qwen3 style plain text thinking tags
	plainThinkRegex := regexp.MustCompile(`(?i)\bthink\b((?s:.*?))\b/think\b`)
	matches = plainThinkRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}

	// Then try to extract from JSON structure
	// This regex looks for "thinking": "content" pattern in JSON
	jsonThinkingRegex := regexp.MustCompile(`(?i)"thinking"\s*:\s*"(.*?)"`)
	matches = jsonThinkingRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		// Unescape any escaped quotes in the JSON string
		thinking := strings.ReplaceAll(matches[1], "\\\"", "\"")
		return thinking
	}

	return ""
}

// ShouldSanitize determines if we should apply internal sanitization based on model and thinking settings
// If thinking=true is sent to server, server handles sanitization so we should skip it
// If thinking=false is sent to server, we handle sanitization internally
func ShouldSanitize(modelName string, enableThinking bool) bool {
    // Skip internal sanitization only when the model itself supports thinking tags
    // AND the caller has explicitly enabled thinking. In that scenario, we assume
    // the server (or middleware) will handle sanitisation appropriately.
    if enableThinking && IsThinkingEnabledModel(modelName) {
        return false
    }

    // In every other case we apply our own sanitisation layer for safety.
    return true
}

// IsThinkingEnabledModel checks if the model is one that supports the thinking tag
func IsThinkingEnabledModel(modelName string) bool {
	modelNameLower := strings.ToLower(modelName)
	return strings.Contains(modelNameLower, "deepseek") ||
		strings.Contains(modelNameLower, "r1-1776") ||
		strings.Contains(modelNameLower, "qwen") ||
		strings.Contains(modelNameLower, "qwen3") ||
		strings.Contains(modelNameLower, "sonar-reasoning") ||
		strings.Contains(modelNameLower, "sonar-reasoning-pro")
}

// ModelResponse represents a structured response from the model with separate thinking and answer fields
type ModelResponse struct {
	Thinking string `json:"thinking"`
	Answer   string `json:"answer"`
}

// ProcessModelOutput attempts to parse structured data and conditionally sanitizes the result
// based on model and thinking settings
func ProcessModelOutput(rawOutput string, modelName string, enableThinking bool) string {
	// If we shouldn't sanitize based on model/settings, return as is
	if !ShouldSanitize(modelName, enableThinking) {
		return rawOutput
	}

	// Try to parse as JSON first (for structured outputs)
	var resp ModelResponse
	if err := json.Unmarshal([]byte(rawOutput), &resp); err == nil {
		// If we successfully parsed JSON and have both thinking and answer
		if resp.Thinking != "" && resp.Answer != "" {
			// If thinking is enabled, return both
			if enableThinking {
				return fmt.Sprintf("<think>%s</think>\n%s", resp.Thinking, resp.Answer)
			}
			// Otherwise just return the answer
			return resp.Answer
		}
	}

	// If JSON parsing failed or didn't have the expected structure,
	// try to extract thinking tags manually and sanitize
	thinking := ExtractThinking(rawOutput)
	if thinking != "" && enableThinking {
		// If thinking is enabled and we found thinking content, reconstruct with proper tags
		sanitized := SanitizeResponse(rawOutput)
		return fmt.Sprintf("<think>%s</think>\n%s", thinking, sanitized)
	}

	// Default case: just sanitize the output
	return SanitizeResponse(rawOutput)
}
