package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
)

// codeFenceRE removes markdown code fences like ```html and ```
var codeFenceRE = regexp.MustCompile("```[a-zA-Z]*\\n?|```")

// Global variable to store reasoning model patterns (can be set from main)
var ReasoningModelPatterns []string

// SetReasoningModelPatterns sets the global list of reasoning model patterns
func SetReasoningModelPatterns(patterns []string) {
	ReasoningModelPatterns = patterns
}

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
	
	// Ensure we have a complete HTML document if the content appears to be HTML
	if strings.Contains(cleaned, "<html") && !strings.Contains(cleaned, "<!DOCTYPE html>") {
		// Add DOCTYPE if missing
		if !strings.HasPrefix(cleaned, "<!") {
			cleaned = "<!DOCTYPE html>\n" + cleaned
		}
	}
	
	// If we don't have any HTML tags at all, wrap the content in a basic HTML document
	if !strings.Contains(cleaned, "<html") && !strings.Contains(cleaned, "<body") {
		// Check if this looks like plain text content that should be wrapped in HTML
		if len(cleaned) > 0 && !strings.HasPrefix(cleaned, "<") {
			// Wrap in a basic HTML document with proper styling
			cleaned = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MuseWeb Response</title>
  <style>
    body { font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif; line-height: 1.6; padding: 1rem; max-width: 800px; margin: 0 auto; }
    pre { background-color: #f5f5f5; padding: 1rem; border-radius: 4px; overflow-x: auto; }
    code { font-family: monospace; background-color: #f5f5f5; padding: 0.2rem 0.4rem; border-radius: 3px; }
  </style>
</head>
<body>
  <div class="content">
    %s
  </div>
</body>
</html>`, strings.ReplaceAll(cleaned, "\n", "<br>\n"))
		}
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
	// Use configurable patterns if available
	if len(ReasoningModelPatterns) > 0 {
		return IsReasoningModel(modelName, ReasoningModelPatterns)
	}
	
	// Fallback to hardcoded patterns for backward compatibility
	// Using priority-based matching: more specific patterns first
	modelNameLower := strings.ToLower(modelName)
	
	// Check specific patterns first
	if strings.Contains(modelNameLower, "deepseek-r1-distill") {
		return true
	}
	if strings.Contains(modelNameLower, "r1-distill") {
		return true
	}
	if strings.Contains(modelNameLower, "sonar-reasoning-pro") {
		return true
	}
	if strings.Contains(modelNameLower, "sonar-reasoning") {
		return true
	}
	if strings.Contains(modelNameLower, "gemini-2.5-flash-lite-preview-06-17") {
		return true
	}
	if strings.Contains(modelNameLower, "gemini-2.5-flash") {
		return true
	}
	if strings.Contains(modelNameLower, "r1-1776") {
		return true
	}
	if strings.Contains(modelNameLower, "qwen3") {
		return true
	}
	
	// Check general patterns last
	if strings.Contains(modelNameLower, "deepseek") {
		return true
	}
	if strings.Contains(modelNameLower, "qwen") {
		return true
	}
	
	return false
}

// IsReasoningModel checks if the model supports reasoning/thinking tags based on a configurable list of patterns
func IsReasoningModel(modelName string, reasoningPatterns []string) bool {
	if len(reasoningPatterns) == 0 {
		// Fallback to the hardcoded function if no patterns are provided
		return IsThinkingEnabledModel(modelName)
	}
	
	modelNameLower := strings.ToLower(modelName)
	
	// Use priority-based matching: check patterns in order and return on first match
	// More specific patterns should be listed first in the configuration
	for _, pattern := range reasoningPatterns {
		if strings.Contains(modelNameLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// ModelResponse represents a structured response from the model with separate thinking and answer fields
type ModelResponse struct {
	Thinking string `json:"thinking"`
	Answer   string `json:"answer"`
}

// ProcessModelOutput attempts to parse structured data and conditionally sanitizes the result
// based on model and thinking settings
func ProcessModelOutput(rawOutput string, modelName string, enableThinking bool) string {
	// Log the raw output length for debugging
	log.Printf("Processing model output: %d bytes from model %s", len(rawOutput), modelName)
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
