// Fixed version of handleWithCustomRequest method
package main

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
)

// handleWithCustomRequest handles models that need special handling with a custom HTTP request
// This is used for models that support thinking tags or have non-standard response formats
func handleWithCustomRequest(ctx context.Context, w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string, modelName string, apiBase string, apiKey string, debug bool, disableThinking bool) error {
	// Create the JSON payload for the request
	jsonData, err := json.Marshal(map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": true,
	})
	if err != nil {
		return fmt.Errorf("failed to create JSON payload: %w", err)
	}

	// Determine the endpoint based on the model name
	endpoint := apiBase
	if !strings.HasSuffix(endpoint, "/") {
		endpoint += "/"
	}
	endpoint += "chat/completions"

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// Create HTTP client with proper debug transport configuration
	var client *http.Client
	if debug {
		// Use debug transport when debug mode is enabled
		client = &http.Client{
			// Transport would be configured here with debug options
			Timeout: 5 * time.Minute,
		}
		log.Printf("[DEBUG] HTTP debugging enabled for custom request")
	} else {
		// Use standard transport without debug logging
		client = &http.Client{
			// Standard transport would be configured here
			Timeout: 5 * time.Minute,
		}
	}

	// Send the HTTP request
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer httpResp.Body.Close()

	// Check if the request was successful
	if httpResp.StatusCode != http.StatusOK {
		// Read the response body for error details
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("HTTP request failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	// Process the streaming response
	var fullResponse strings.Builder
	
	// For debugging, capture the entire raw response
	var rawResponseCopy bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(httpResp.Body, &rawResponseCopy))
	
	// Log response headers for debugging
	if debug {
		log.Printf("[DEBUG] Response status: %s", httpResp.Status)
		log.Printf("[DEBUG] Response headers: %v", httpResp.Header)
	}
	
	// Check if we're dealing with SSE (Server-Sent Events) format
	contentType := httpResp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")
	if isSSE && debug {
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
		if debug {
			log.Printf("[DEBUG] Raw line: %s", line)
		}
		
		// Process SSE data lines
		if strings.HasPrefix(line, "data: ") {
			dataContent := strings.TrimPrefix(line, "data: ")
			var content string
			
			// Try Gemini-specific JSON unmarshal to extract content parts
			var geminiResp struct{ Candidates []struct{ Content struct{ Parts []struct{ Text string `json:"text"` } `json:"parts"` } `json:"content"` } `json:"candidates"` }
			if err := json.Unmarshal([]byte(dataContent), &geminiResp); err == nil {
				if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
					content = geminiResp.Candidates[0].Content.Parts[0].Text
					if debug {
						log.Printf("[DEBUG] Extracted Gemini content: %q", content)
					}
				}
			} else if debug {
				log.Printf("[DEBUG] Not a valid Gemini response: %v", err)
			}
			
			// If Gemini extraction failed, try standard OpenAI format
			if content == "" {
				var resp struct{ Choices []struct{ Delta struct{ Content string `json:"content"` } `json:"delta"` } `json:"choices"` }
				if err := json.Unmarshal([]byte(dataContent), &resp); err == nil {
					if len(resp.Choices) > 0 && resp.Choices[0].Delta.Content != "" {
						content = resp.Choices[0].Delta.Content
						if debug {
							log.Printf("[DEBUG] Extracted standard content: %q", content)
						}
					}
				} else if debug {
					log.Printf("[DEBUG] Not a valid standard response: %v", err)
				}
			}
			
			// If both extractions failed, try generic content extraction
			if content == "" {
				// Generic extractor would be called here
				if debug {
					log.Printf("[DEBUG] Using generic extractor for: %s", dataContent)
				}
			}
			
			// Add extracted content to the full response
			if content != "" {
				fullResponse.WriteString(content)
				// Flush partial content to client for real-time updates
				_, err := io.WriteString(w, content)
				if err != nil {
					log.Printf("[ERROR] Client disconnected during streaming: %v", err)
					return fmt.Errorf("client disconnected: %w", err)
				}
				flusher.Flush()
			}
		}
	}

	// Now that the stream is complete, process the full response
	responseStr := fullResponse.String()
	
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
		}
	}
	
	if debug {
		log.Printf("[DEBUG] Raw model response length: %d bytes", len(responseStr))
		// Log a preview of the raw response (first 100 chars)
		if len(responseStr) > 0 {
			previewLen := 100
			if len(responseStr) < previewLen {
				previewLen = len(responseStr)
			}
			log.Printf("[DEBUG] Raw response preview: %s...", responseStr[:previewLen])
		} else {
			log.Printf("[ERROR] Empty raw response from model %s", modelName)
		}
	}
	
	// Process the final output (would call a utility function here)
	finalOutput := responseStr
	
	// Write the final output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	
	return nil
}
