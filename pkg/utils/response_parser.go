package utils

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
)

// ContentWrapper represents the non-standard content format some providers return
type ContentWrapper struct {
	String string      `json:"String"`
	Array  interface{} `json:"Array"`
}

// ResponseChoice represents a single choice in a response
type ResponseChoice struct {
	Delta struct {
		Content interface{} `json:"content"` // Can be string or ContentWrapper
	} `json:"delta"`
}

// ExtractContentFromResponse extracts content from a raw JSON response chunk
// ExtractContentFromResponse attempts to extract content from both standard and non-standard response formats
func ExtractContentFromResponse(jsonStr string) string {
	// Log the raw JSON for debugging
	log.Printf("[DEBUG] Extracting content from: %s", jsonStr)
	// Try to parse the JSON as a map
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonMap); err != nil {
		log.Printf("[DEBUG] Failed to parse JSON: %v", err)
		// Try to extract content from non-JSON data
		if strings.Contains(jsonStr, "text") {
			re := regexp.MustCompile(`"text"\s*:\s*"(.*?)"`) 
			matches := re.FindStringSubmatch(jsonStr)
			if len(matches) > 1 {
				log.Printf("[DEBUG] Extracted text using regex: %s", matches[1])
				return matches[1]
			}
		}
		return ""
	}

	// Check if there's a choices array
	choices, ok := jsonMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}

	// Check the first choice
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}

	// Check for delta or message
	var contentContainer map[string]interface{}
	if delta, ok := choice["delta"].(map[string]interface{}); ok {
		contentContainer = delta
	} else if message, ok := choice["message"].(map[string]interface{}); ok {
		contentContainer = message
	} else {
		return ""
	}

	// First check if content is a direct string (standard format)
	if content, ok := contentContainer["content"].(string); ok && content != "" {
		return content
	}

	// Then check if content is an object with a String field (non-standard format)
	if contentObj, ok := contentContainer["content"].(map[string]interface{}); ok {
		// Try String field first (common in some models)
		if strContent, ok := contentObj["String"].(string); ok {
			return strContent
		}
		
		// Try text field (used by some models like Gemini)
		if textContent, ok := contentObj["text"].(string); ok {
			return textContent
		}
		
		// Try parts array (used by some models)
		if parts, ok := contentObj["parts"].([]interface{}); ok && len(parts) > 0 {
			if textPart, ok := parts[0].(string); ok {
				return textPart
			} else if partMap, ok := parts[0].(map[string]interface{}); ok {
				if text, ok := partMap["text"].(string); ok {
					return text
				}
			}
		}
	}

	// Log the full content container for debugging
	contentJSON, _ := json.Marshal(contentContainer)
	log.Printf("[DEBUG] Content container structure: %s", string(contentJSON))

	// Check for direct text field at the top level (some models use this)
	if textContent, ok := contentContainer["text"].(string); ok {
		return textContent
	}
	
	// Check for parts array at the top level (some models use this)
	if parts, ok := contentContainer["parts"].([]interface{}); ok && len(parts) > 0 {
		if textPart, ok := parts[0].(string); ok {
			return textPart
		} else if partMap, ok := parts[0].(map[string]interface{}); ok {
			if text, ok := partMap["text"].(string); ok {
				return text
			}
		}
	}
	
	// Last resort: try to marshal the content back to JSON and extract using regex
	contentJSON, err := json.Marshal(contentContainer["content"])
	if err != nil {
		return ""
	}

	// Use regex to extract the String field value
	re := regexp.MustCompile(`"String"\s*:\s*"(.*?)"`)
	matches := re.FindStringSubmatch(string(contentJSON))
	if len(matches) > 1 {
		// Unescape JSON string escapes
		unescaped := strings.ReplaceAll(matches[1], "\\\"", "\"")
		unescaped = strings.ReplaceAll(unescaped, "\\\\", "\\")
		unescaped = strings.ReplaceAll(unescaped, "\\n", "\n")
		unescaped = strings.ReplaceAll(unescaped, "\\t", "\t")
		unescaped = strings.ReplaceAll(unescaped, "\\r", "\r")
		unescaped = strings.ReplaceAll(unescaped, "\\u003c", "<")
		unescaped = strings.ReplaceAll(unescaped, "\\u003e", ">")
		unescaped = strings.ReplaceAll(unescaped, "\\u0026", "&")
		return unescaped
	}

	return ""
}

// UnwrapContentStringField replaces {"content": {"String": "...", "Array": null}} with {"content": "..."}
// This is useful for processing complete (non-streaming) responses
func UnwrapContentStringField(raw string) string {
	// Regex to match: "content":\s*{\s*"String":\s*"(...)",\s*"Array":\s*null\s*}
	re := regexp.MustCompile(`"content"\s*:\s*{\s*"String"\s*:\s*"(.*?)"\s*,\s*"Array"\s*:\s*null\s*}`)
	
	// Replace with: "content": "<value>"
	// We need to handle escaped quotes in the captured content
	return re.ReplaceAllStringFunc(raw, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) > 1 {
			// Keep the JSON escaping intact
			return `"content":"` + submatches[1] + `"`
		}
		return match
	})
}
