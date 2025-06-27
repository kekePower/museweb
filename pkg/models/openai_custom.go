package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kekePower/museweb/pkg/utils"
)

// handleWithCustomRequest handles models that need special handling with a custom HTTP request
// This is used for models that support thinking tags or have non-standard response formats
// extractTextFromMap recursively searches for text or content fields in a map structure
func extractTextFromMap(m map[string]interface{}, debug bool) string {
	// Look for common text field names
	for _, key := range []string{"text", "content", "value", "message"} {
		if val, ok := m[key]; ok {
			// If we found a string value, return it
			if strVal, ok := val.(string); ok && strVal != "" {
				if debug {
					log.Printf("[DEBUG] Found text in field %q: %q", key, strVal)
				}
				return strVal
			}
		}
	}

	// Recursively check all map values
	for _, val := range m {
		switch v := val.(type) {
		case map[string]interface{}:
			// Recursively search nested maps
			if result := extractTextFromMap(v, debug); result != "" {
				return result
			}
		case []interface{}:
			// Search through array elements
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if result := extractTextFromMap(itemMap, debug); result != "" {
						return result
					}
				} else if strItem, ok := item.(string); ok && strItem != "" {
					// If this is an array of strings, check if any look like content
					if len(strItem) > 5 && !strings.HasPrefix(strItem, "http") {
						if debug {
							log.Printf("[DEBUG] Found text in array item: %q", strItem)
						}
						return strItem
					}
				}
			}
		}
	}

	// No text content found
	return ""
}

// Global variable to track how much content we've already sent from the buffer
var lastSentLength int

// processStreamingContent uses incremental buffer cleaning for cross-chunk pattern handling
// while maintaining real-time streaming experience
func processStreamingContent(newContent string, pendingBuffer *strings.Builder) string {
	// Add new content to pending buffer
	pendingBuffer.WriteString(newContent)
	bufferContent := pendingBuffer.String()
	
	// Check if we've seen </html> - this indicates HTML content is complete
	htmlEndPos := strings.Index(strings.ToLower(bufferContent), "</html>")
	
	if htmlEndPos == -1 {
		// No </html> found yet - use incremental buffer cleaning
		// Clean the entire buffer (handles cross-chunk patterns)
		cleanedBuffer := utils.CleanupCodeFences(bufferContent)
		
		// Only send the new portion that hasn't been sent yet
		if len(cleanedBuffer) > lastSentLength {
			newContent := cleanedBuffer[lastSentLength:]
			lastSentLength = len(cleanedBuffer)
			return newContent
		}
		
		// No new content to send
		return ""
		
	} else {
		// We found </html>! HTML document is complete.
		// Remove EVERYTHING after </html> to eliminate LLM chatter
		htmlEndTag := "</html>"
		htmlEndFull := htmlEndPos + len(htmlEndTag)
		
		// Only keep content up to and including </html>
		beforeAndIncluding := bufferContent[:htmlEndFull]
		
		// Clean the complete HTML content (handles all cross-chunk patterns)
		cleanedContent := utils.CleanupCodeFences(beforeAndIncluding)
		
		// Calculate what new content to send (difference from what we've sent so far)
		if len(cleanedContent) > lastSentLength {
			newContent := cleanedContent[lastSentLength:]
			lastSentLength = len(cleanedContent)
			
			// Clear the pending buffer since we're done
			pendingBuffer.Reset()
			lastSentLength = 0 // Reset for next request
			
			return newContent
		}
		
		// Clear the pending buffer since we're done
		pendingBuffer.Reset()
		lastSentLength = 0 // Reset for next request
		return ""
	}
}

func (h *OpenAIHandler) handleWithCustomRequest(ctx context.Context, w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	// Using standard OpenAI API format for all models

	// Create the JSON payload for the request using standard OpenAI format for all models
	payload := map[string]interface{}{
		"model": h.ModelName,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": true,
	}

	// For reasoning models, always disable thinking to avoid reasoning output in web pages
	if utils.IsReasoningModel(h.ModelName, utils.ReasoningModelPatterns) {
		payload["thinking"] = false
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error creating JSON payload: %w", err)
	}

	if h.Debug {
		log.Printf("🔍 Outgoing JSON payload for %s:\n%s", h.ModelName, string(jsonData))
	}

	// Use standard OpenAI API endpoint for all models
	endpoint := "/chat/completions"

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.APIBase+endpoint,
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("error creating HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if h.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	// Create HTTP client with proper timeout
	var httpClient *http.Client
	if h.Debug {
		// Use debug transport when debug mode is enabled
		httpClient = &http.Client{
			Transport: &utils.DebugTransport{
				Transport: http.DefaultTransport,
			},
			Timeout: 5 * time.Minute,
		}
		log.Printf("[DEBUG] HTTP debugging enabled for custom request")
	} else {
		// Use standard transport without debug logging
		httpClient = &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   5 * time.Minute,
		}
	}

	// Send request
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer httpResp.Body.Close()

	// Check response status
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("error from API: %s - %s", httpResp.Status, string(body))
	}

	// Process the streaming response
	var fullResponse strings.Builder
	
	// Smart streaming buffer for pattern detection
	var streamBuffer strings.Builder
	var pendingBuffer strings.Builder  // Holds content that might be part of a fence

	// For debugging, capture the entire raw response
	var rawResponseCopy bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(httpResp.Body, &rawResponseCopy))

	// Log response headers for debugging
	if h.Debug {
		log.Printf("[DEBUG] Response status: %s", httpResp.Status)
		log.Printf("[DEBUG] Response headers: %v", httpResp.Header)
	}

	// Check if we're dealing with SSE (Server-Sent Events) format
	contentType := httpResp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")
	if isSSE && h.Debug {
		log.Printf("[DEBUG] Detected SSE (Server-Sent Events) format")
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response: %w", err)
		}

		// Skip empty lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip "data: [DONE]" messages
		if line == "data: [DONE]" {
			continue
		}

		// Log the raw line for debugging
		if h.Debug {
			log.Printf("[DEBUG] Raw line: %s", line)
		}

		// Process SSE data lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var content string

			// Try Gemini-specific JSON unmarshal to extract content parts
			// First try the standard Gemini format
			var geminiResp struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal([]byte(data), &geminiResp); err == nil {
				if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
					content = geminiResp.Candidates[0].Content.Parts[0].Text
					if h.Debug {
						log.Printf("[DEBUG] Extracted Gemini content: %q", content)
					}
				}
			} else if h.Debug {
				log.Printf("[DEBUG] Not a valid standard Gemini response: %v", err)

				// Try alternative Gemini response format
				var altGeminiResp map[string]interface{}
				if err := json.Unmarshal([]byte(data), &altGeminiResp); err == nil {
					if candidates, ok := altGeminiResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
						if candidate, ok := candidates[0].(map[string]interface{}); ok {
							if contentObj, ok := candidate["content"].(map[string]interface{}); ok {
								if parts, ok := contentObj["parts"].([]interface{}); ok && len(parts) > 0 {
									if part, ok := parts[0].(map[string]interface{}); ok {
										if text, ok := part["text"].(string); ok {
											content = text
											if h.Debug {
												log.Printf("[DEBUG] Extracted alternative Gemini content: %q", content)
											}
										}
									}
								}
							}
						}
					}
				}
			}

			// If Gemini extraction failed, try standard OpenAI format
			if content == "" {
				var resp struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				if err := json.Unmarshal([]byte(data), &resp); err == nil {
					if len(resp.Choices) > 0 && resp.Choices[0].Delta.Content != "" {
						content = resp.Choices[0].Delta.Content
						if h.Debug {
							log.Printf("[DEBUG] Extracted standard content: %q", content)
						}
					}
				} else if h.Debug {
					log.Printf("[DEBUG] Not a valid standard response: %v", err)
				}
			}

			// If both extractions failed, try generic content extraction
			if content == "" {
				// Try to extract content from the JSON payload
				content := utils.ExtractContentFromResponse(data)

				// If standard extraction failed, try recursive extraction
				if content == "" {
					// Try to parse the JSON data
					var anyJson map[string]interface{}
					if err := json.Unmarshal([]byte(data), &anyJson); err == nil {
						// Recursively search for text content
						content = extractTextFromMap(anyJson, h.Debug)
						if content != "" && h.Debug {
							log.Printf("[DEBUG] Found text content via deep search: %q", content)
						}
					} else if h.Debug {
						log.Printf("[DEBUG] JSON parsing failed: %v", err)
					}

					// If still no content, try the raw line as a last resort
					if content == "" && len(data) > 0 && !strings.HasPrefix(data, "{") {
						content = data
						if h.Debug {
							log.Printf("[DEBUG] Using raw data as content: %d bytes", len(content))
						}
					}
				}
			}

			// Smart streaming with pattern detection
			if content != "" {
				fullResponse.WriteString(content)
				streamBuffer.WriteString(content)
				
				// Process the content for real-time streaming with fence detection
				processedContent := processStreamingContent(content, &pendingBuffer)
				
				// Send processed content to client immediately (real-time streaming)
				if processedContent != "" {
					_, err := io.WriteString(w, processedContent)
					if err != nil {
						log.Printf("[ERROR] Client disconnected during streaming: %v", err)
						return fmt.Errorf("client disconnected: %w", err)
					}
					flusher.Flush()
				}
				
				if h.Debug {
					log.Printf("[DEBUG] Streamed content chunk: %d bytes (processed: %d bytes)", len(content), len(processedContent))
				}
			}
		}
	}

	// Now that the stream is complete, flush any remaining pending content
	responseStr := fullResponse.String()
	
	// Flush any remaining content in the pending buffer
	if pendingBuffer.Len() > 0 {
		// Apply final cleanup to any remaining pending content
		// At end of stream, be more aggressive about removing trailing artifacts
		finalPending := utils.CleanupCodeFences(pendingBuffer.String())
		
		// Additional end-of-stream cleanup for any remaining backticks
		finalPending = strings.TrimSpace(finalPending)
		if strings.HasSuffix(finalPending, "```") {
			finalPending = strings.TrimSuffix(finalPending, "```")
			finalPending = strings.TrimSpace(finalPending)
		}
		
		if finalPending != "" {
			_, err = io.WriteString(w, finalPending)
			if err != nil {
				log.Printf("[ERROR] Failed to send final pending content: %v", err)
			} else {
				flusher.Flush()
			}
		}
		
		if h.Debug {
			log.Printf("[DEBUG] Flushed final pending content: %d bytes", len(finalPending))
		}
	}

	// If we got no content from the stream processing, log the raw response
	if len(responseStr) == 0 {
		log.Printf("[ERROR] No content extracted from streaming. Raw response dump:")
		rawResponseStr := rawResponseCopy.String()
		if len(rawResponseStr) > 0 {
			// Log the raw response in chunks to avoid truncation
			for i := 0; i < len(rawResponseStr); i += 1000 {
				end := i + 1000
				if end > len(rawResponseStr) {
					end = len(rawResponseStr)
				}
				log.Printf("[RAW RESPONSE] %s", rawResponseStr[i:end])
			}

			// Try to extract content directly from the raw response
			rawLines := strings.Split(rawResponseStr, "\n")
			for _, line := range rawLines {
				if strings.HasPrefix(line, "data: ") {
					// Extract the JSON data
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						continue
					}

					// Try to extract content from the response
					content := ""

					// Parse standard OpenAI API response format first (works for all OpenAI-compatible APIs)
					var openAIResp struct {
						ID      string `json:"id"`
						Object  string `json:"object"`
						Created int64  `json:"created"`
						Model   string `json:"model"`
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
								Role    string `json:"role"`
							} `json:"delta"`
							Index        int    `json:"index"`
							FinishReason string `json:"finish_reason"`
						} `json:"choices"`
					}

					if err := json.Unmarshal([]byte(data), &openAIResp); err == nil {
						if len(openAIResp.Choices) > 0 && openAIResp.Choices[0].Delta.Content != "" {
							content = openAIResp.Choices[0].Delta.Content
							if h.Debug {
								log.Printf("[DEBUG] Successfully extracted OpenAI content: %q", content)
							}
						}
					} else if h.Debug {
						log.Printf("[DEBUG] Failed to parse standard OpenAI format: %v", err)
					}

					// If standard parsing failed, try the generic extractor
					if content == "" {
						content = utils.ExtractContentFromResponse(data)
						if content != "" && h.Debug {
							log.Printf("[DEBUG] Extracted content using generic extractor: %d bytes", len(content))
						}
					}

					if content != "" {
						fullResponse.WriteString(content)
						// Send the content to the client
						fmt.Fprintf(w, "%s", content)
						flusher.Flush()
					}
				}
			}

			// Update the raw response with any newly extracted content
			responseStr = fullResponse.String()
		} else {
			log.Printf("[ERROR] Empty raw response capture")
		}
	}

	if h.Debug {
		log.Printf("[DEBUG] Streaming complete. Total response length: %d bytes", len(responseStr))
	}

	return nil
}
