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
func SanitizeResponse(s string, modelName string, enableThinking bool) string {
	// Input should already have code fences cleaned by ProcessModelOutput
	cleaned := s

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

	// Handle orphaned "html" text that appears alone on a line (from code fence removal)
	// Be very specific to avoid removing legitimate HTML content
	lines := strings.Split(cleaned, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		// Only remove if the first line is EXACTLY "html" and nothing else
		if firstLine == "html" || firstLine == "HTML" {
			// Remove the first line containing only "html"
			lines = lines[1:]
			cleaned = strings.Join(lines, "\n")
		}
	}
	
	// Fix common DOCTYPE issues where the opening < got removed
	if strings.HasPrefix(strings.TrimSpace(cleaned), "!DOCTYPE") {
		cleaned = strings.TrimSpace(cleaned)
		cleaned = "<" + cleaned
	} else if strings.HasPrefix(strings.TrimSpace(cleaned), "html") {
		// Only add < if this looks like a legitimate HTML tag (contains attributes or >)
		trimmed := strings.TrimSpace(cleaned)
		if strings.Contains(trimmed, ">") || strings.Contains(trimmed, " ") {
			cleaned = "<" + trimmed
		}
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
%s
</body>
</html>`, cleaned)
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
	if strings.Contains(modelNameLower, "mercury-coder") {
		return true
	}
	if strings.Contains(modelNameLower, "mercury") {
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
	
	// ALWAYS clean up code fences first - this is about markdown artifacts, not thinking content
	cleaned := CleanupCodeFences(rawOutput)
	cleaned = codeFenceRE.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "`", "")
	
	// If we shouldn't sanitize thinking-related content, return the code-fence-cleaned version
	if !ShouldSanitize(modelName, enableThinking) {
		return cleaned
	}

	// Try to parse as JSON first (for structured outputs)
	var resp ModelResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err == nil {
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
	thinking := ExtractThinking(cleaned)
	if thinking != "" && enableThinking {
		// If thinking is enabled and we found thinking content, reconstruct with proper tags
		sanitized := SanitizeResponse(cleaned, modelName, enableThinking)
		return fmt.Sprintf("<think>%s</think>\n%s", thinking, sanitized)
	}

	// Default case: just sanitize the output
	return SanitizeResponse(cleaned, modelName, enableThinking)
}

// CleanupCodeFences removes markdown code fence patterns with surgical precision
// to avoid accidentally removing legitimate HTML content
// This function is designed to work with ANY prompt set and AI output format
// Optimized with pre-checks to avoid expensive regex operations when not needed
func CleanupCodeFences(s string) string {
	output := s
	

	
	// Step 0: Universal HTML extraction - handle AI responses with explanatory text
	// This ensures we extract clean HTML regardless of prompt instructions or backticks
	if strings.Contains(output, "<!DOCTYPE") {
		// Find the start of the HTML document
		doctypePos := strings.Index(output, "<!DOCTYPE")
		if doctypePos > 0 {
			// Remove everything before DOCTYPE (explanatory text, etc.)
			output = output[doctypePos:]
		}
		
		// Find the end of the HTML document
		htmlEndPos := strings.LastIndex(strings.ToLower(output), "</html>")
		if htmlEndPos != -1 {
			// Remove everything after </html>
			htmlEndFull := htmlEndPos + len("</html>")
			output = output[:htmlEndFull]
		}
	} else if strings.Contains(output, "<html") {
		// Handle HTML without DOCTYPE
		htmlStartPos := strings.Index(output, "<html")
		if htmlStartPos > 0 {
			// Remove everything before <html
			output = output[htmlStartPos:]
		}
		
		// Find the end of the HTML document
		htmlEndPos := strings.LastIndex(strings.ToLower(output), "</html>")
		if htmlEndPos != -1 {
			// Remove everything after </html>
			htmlEndFull := htmlEndPos + len("</html>")
			output = output[:htmlEndFull]
		}
	}
	

	
	// Early return if no backticks present - most common case for clean HTML
	if !strings.Contains(output, "`") {
		return output
	}
	
	// Step 1: Remove common code fence patterns with direct string operations (fastest)
	// Enhanced to handle various AI output formats from different prompt sets
	output = strings.ReplaceAll(output, "```html\n", "")
	output = strings.ReplaceAll(output, "```HTML\n", "")
	output = strings.ReplaceAll(output, "```html", "")
	output = strings.ReplaceAll(output, "```HTML", "")
	// Handle other common fence variations
	output = strings.ReplaceAll(output, "```xml\n", "")
	output = strings.ReplaceAll(output, "```xml", "")
	output = strings.ReplaceAll(output, "```markup\n", "")
	output = strings.ReplaceAll(output, "```markup", "")
	// Handle generic fences
	output = strings.ReplaceAll(output, "```\n", "")
	output = strings.ReplaceAll(output, "```", "")
	
	// Step 2: Handle orphaned "html" at the very beginning
	// This is the most common leftover from ```html removal
	// Be very precise to avoid removing legitimate HTML content
	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		// Only remove if the first line is EXACTLY "html" or "HTML" and nothing else
		// This avoids removing legitimate HTML tags like "<html lang='en'>"
		if firstLine == "html" || firstLine == "HTML" {
			lines = lines[1:] // Remove the first line containing only "html"
			output = strings.Join(lines, "\n")
		}
	}
	
	// Step 3: Handle inline code backticks (preserve content, remove backticks)
	// Only run if single backticks are present (no triple backticks should remain)
	// Be very conservative to avoid breaking HTML tags
	if strings.Contains(output, "`") && !strings.Contains(output, "```") {
		// Only process single backticks that don't contain HTML-like content
		// Avoid matching patterns that might contain < or > characters
		inlineCodeReg := regexp.MustCompile("`([^`\n<>]+)`")
		output = inlineCodeReg.ReplaceAllString(output, "$1")
	}
	
	// Step 4: Clean up excessive whitespace
	// Replace multiple consecutive newlines with maximum of 2 newlines
	if strings.Contains(output, "\n\n\n") {
		multipleNewlinesReg := regexp.MustCompile(`\n{3,}`)
		output = multipleNewlinesReg.ReplaceAllString(output, "\n\n")
	}
	
	// Step 5: Handle trailing backticks at the very end (common in streaming)
	// This catches cases where ``` or single ` appears at the end with potential whitespace
	output = strings.TrimSpace(output)
	if strings.HasSuffix(output, "```") {
		output = strings.TrimSuffix(output, "```")
		output = strings.TrimSpace(output) // Clean up any trailing whitespace after removal
	} else if strings.HasSuffix(output, "`") {
		// Handle single trailing backtick (common when ``` gets partially removed)
		output = strings.TrimSuffix(output, "`")
		output = strings.TrimSpace(output) // Clean up any trailing whitespace after removal
	}
	
	
	// Step 7: Final cleanup - remove leading/trailing empty lines
	finalLines := strings.Split(output, "\n")
	start := 0
	for start < len(finalLines) && strings.TrimSpace(finalLines[start]) == "" {
		start++
	}
	end := len(finalLines)
	for end > start && strings.TrimSpace(finalLines[end-1]) == "" {
		end--
	}
	if start < end {
		output = strings.Join(finalLines[start:end], "\n")
	} else {
		output = ""
	}
	

	
	return output
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
