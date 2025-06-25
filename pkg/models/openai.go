package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kekePower/museweb/pkg/utils"
	openai "github.com/sashabaranov/go-openai"
)

// OpenAIHandler implements the ModelHandler interface for OpenAI-compatible APIs
type OpenAIHandler struct {
	ModelName       string
	APIKey          string
	APIBase         string
	DisableThinking bool
	Debug           bool
}

// StreamResponse streams the response from the OpenAI model
func (h *OpenAIHandler) StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	ctx := context.Background()

	// Create the OpenAI client
	config := openai.DefaultConfig(h.APIKey)
	config.BaseURL = h.APIBase

	// Add custom transport with conditional debugging and thinking tag support
	if h.Debug {
		// Use debug transport when debug mode is enabled
		config.HTTPClient = &http.Client{
			Transport: &utils.DebugTransport{
				Transport: &customHeaderTransport{
					base:     http.DefaultTransport,
					thinking: !h.DisableThinking,
				},
			},
			Timeout: 5 * time.Minute,
		}
		log.Printf("[DEBUG] HTTP debugging enabled for OpenAI client")
	} else {
		// Use standard transport without debug logging when debug mode is disabled
		config.HTTPClient = &http.Client{
			Transport: &customHeaderTransport{
				base:     http.DefaultTransport,
				thinking: !h.DisableThinking,
			},
			Timeout: 5 * time.Minute,
		}
	}

	client := openai.NewClientWithConfig(config)

	// Create the chat completion request
	req := openai.ChatCompletionRequest{
		Model: h.ModelName,
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: true,
	}

	// Use the OpenAI SDK for all models since the server is now sending 100% OpenAI API compatible responses
	if h.Debug {
		log.Printf("[DEBUG] Creating OpenAI stream with model: %s, API base: %s", h.ModelName, h.APIBase)
	}

	// Only use handleWithCustomRequest if thinking tags are enabled for this model
	if !h.DisableThinking && utils.IsThinkingEnabledModel(h.ModelName) {
		if h.Debug {
			log.Printf("[DEBUG] Using custom request handler for thinking tags with model: %s", h.ModelName)
		}
		return h.handleWithCustomRequest(ctx, w, flusher, systemPrompt, userPrompt)
	}

	// Log the request details for debugging
	if h.Debug {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		log.Printf("[DEBUG] OpenAI request payload:\n%s", string(reqJSON))
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.Printf("[ERROR] Failed to start OpenAI stream: %v (type: %T)", err, err)

		// Try to extract more details from the error
		if apiErr, ok := err.(*openai.APIError); ok {
			log.Printf("[ERROR] OpenAI API error: Status=%d, Type=%s, Message=%s",
				apiErr.HTTPStatusCode, apiErr.Type, apiErr.Message)

			// Provide more specific error messages based on status code
			switch apiErr.HTTPStatusCode {
			case 401:
				log.Printf("[ERROR] Authentication error. Check your API key and permissions")
			case 404:
				log.Printf("[ERROR] Model not found or API endpoint incorrect. Check model name and API base URL")
			case 429:
				log.Printf("[ERROR] Rate limit exceeded or quota reached")
			case 400:
				log.Printf("[ERROR] Bad request. The API rejected the request parameters")
			default:
				if apiErr.HTTPStatusCode >= 500 {
					log.Printf("[ERROR] Server error from provider. The service may be experiencing issues")
				}
			}
		}

		// Try to make a direct HTTP request to get more error details
		log.Printf("[DEBUG] Attempting direct HTTP request as fallback...")
		rawResp, rawErr := tryDirectRequest(h.APIBase, h.APIKey, h.ModelName, systemPrompt, userPrompt, h.Debug)
		if rawErr == nil && rawResp != "" {
			log.Printf("[DEBUG] Raw API response from fallback request: %s", rawResp)
			// Try to write this to the client as a fallback
			_, writeErr := io.WriteString(w, rawResp)
			if writeErr == nil {
				flusher.Flush()
				return nil
			}
		} else if rawErr != nil {
			log.Printf("[ERROR] Fallback request also failed: %v", rawErr)
		}

		return fmt.Errorf("failed to start OpenAI stream: %w", err)
	}
	defer stream.Close()

	// Use a strings.Builder to buffer the full response
	var fullResponse strings.Builder
	streamingFailed := false

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.Printf("[DEBUG] OpenAI stream completed normally with EOF")
			break
		}
		if err != nil {
			log.Printf("[ERROR] OpenAI stream error: %v (%T)", err, err)

			// Try to extract more details from the error
			if unwrapped := errors.Unwrap(err); unwrapped != nil {
				log.Printf("[ERROR] Unwrapped error: %v (%T)", unwrapped, unwrapped)
			}

			// Check if it's an API error
			if apiErr, ok := err.(*openai.APIError); ok {
				log.Printf("[ERROR] OpenAI API error: Status=%d, Type=%s, Message=%s",
					apiErr.HTTPStatusCode, apiErr.Type, apiErr.Message)
				// Error handling is already done in the initial stream creation
			}

			rawJSON, err := json.Marshal(response)
			if err == nil && len(rawJSON) > 2 { // More than just '{}'
				log.Printf("[DEBUG] Last response before error: %s", string(rawJSON))
			}

			// Mark streaming as failed and break to try fallback
			streamingFailed = true
			break
		}

		// Process the streaming chunk
		if len(response.Choices) > 0 {
			// Extract content from the standard OpenAI format
			chunkContent := response.Choices[0].Delta.Content

			if h.Debug {
				log.Printf("[DEBUG] Stream chunk: %q", chunkContent)
			}

			if chunkContent != "" {
				// Add to full response
				fullResponse.WriteString(chunkContent)

				// Write to client and flush
				_, err := io.WriteString(w, chunkContent)
				if err != nil {
					log.Printf("[ERROR] Failed to write to client: %v", err)
					break
				}
				flusher.Flush()
			} else if h.Debug {
				// If we got an empty content delta, log the raw response for debugging
				rawJSON, _ := json.Marshal(response)
				log.Printf("[DEBUG] Empty content delta. Raw chunk: %s", string(rawJSON))
			}
		} else {
			// If there are no choices, try to extract content from the response
			respJSON, err := json.Marshal(response)
			if err != nil {
				log.Printf("[ERROR] Failed to marshal response to JSON: %v", err)
				break
			}
			content := utils.ExtractContentFromResponse(string(respJSON))
			if content != "" {
				log.Printf("[DEBUG] Extracted content from non-streaming response: %q", content)
				fullResponse.WriteString(content)

				// Write to client and flush
				_, err := io.WriteString(w, content)
				if err != nil {
					log.Printf("[ERROR] Failed to write to client: %v", err)
					break
				}
				flusher.Flush()
			}
		}
	}

	// If streaming failed and we have no response, try the fallback
	if streamingFailed && fullResponse.Len() == 0 {
		log.Printf("[DEBUG] Streaming failed with no content, attempting non-streaming fallback...")
		
		// Try to make a non-streaming request as fallback
		fallbackResp, fallbackErr := h.tryNonStreamingRequest(ctx, systemPrompt, userPrompt)
		if fallbackErr == nil && fallbackResp != "" {
			log.Printf("[DEBUG] Non-streaming fallback successful, response length: %d bytes", len(fallbackResp))
			fullResponse.WriteString(fallbackResp)
			
			// Write the fallback response to the client
			_, writeErr := io.WriteString(w, fallbackResp)
			if writeErr != nil {
				log.Printf("[ERROR] Failed to write fallback response to client: %v", writeErr)
				return writeErr
			}
			flusher.Flush()
		} else {
			log.Printf("[ERROR] Non-streaming fallback also failed: %v", fallbackErr)
			
			// Check if the error is an I/O timeout and we have partial content
			if strings.Contains(fallbackErr.Error(), "i/o timeout") || strings.Contains(fallbackErr.Error(), "EOF") {
				log.Printf("[DEBUG] Detected I/O timeout/EOF error, checking for partial content...")
				
				// If we have any partial content from streaming, use it
				if fullResponse.Len() > 0 {
					log.Printf("[WARNING] Using partial response due to server I/O timeout. Length: %d bytes", fullResponse.Len())
					// Don't return an error - we have some content to work with
				} else {
					return fmt.Errorf("server I/O timeout with no partial content available")
				}
			} else {
				return fmt.Errorf("both streaming and non-streaming requests failed")
			}
		}
	} else if streamingFailed && fullResponse.Len() > 0 {
		// We have partial content from streaming failure
		responseStr := fullResponse.String()
		log.Printf("[WARNING] Streaming failed but recovered partial response. Length: %d bytes", fullResponse.Len())
		
		// Check if the response looks incomplete (e.g., doesn't end with </html>)
		if strings.Contains(responseStr, "<!DOCTYPE html") {
			if !strings.HasSuffix(strings.TrimSpace(responseStr), "</html>") {
				log.Printf("[WARNING] HTML response appears incomplete - missing closing </html> tag")
				log.Printf("[DEBUG] Response ends with: %q", responseStr[len(responseStr)-min(100, len(responseStr)):])
			} else {
				log.Printf("[INFO] HTML response appears complete despite streaming failure")
			}
		}
	}

	responseStr := fullResponse.String()

	// Log completion and response statistics
	if h.Debug {
		log.Printf("[DEBUG] Streaming complete. Full response length: %d bytes", len(responseStr))
		if len(responseStr) > 0 {
			// Log the first 100 characters of the response for debugging
			previewLen := 100
			if len(responseStr) < previewLen {
				previewLen = len(responseStr)
			}
			log.Printf("[DEBUG] Response preview: %q", responseStr[:previewLen])
		} else {
			log.Printf("[WARNING] Received empty response from provider")
		}
	}

	// We've already streamed the content to the client in real-time,
	// so we only need to process any final output if needed
	if !h.DisableThinking && utils.ShouldSanitize(h.ModelName, !h.DisableThinking) {
		// Only process the output if thinking tags are enabled and the model needs sanitization
		finalOutput := utils.ProcessModelOutput(responseStr, h.ModelName, true)

		// Check if processing actually changed anything
		if finalOutput != responseStr {
			if h.Debug {
				log.Printf("[DEBUG] Processed final output with thinking tags")
			}

			// Write the final, clean output to the client
			_, writeErr := io.WriteString(w, finalOutput)
			if writeErr != nil {
				log.Printf("Client disconnected before final write.")
				return writeErr
			}
			flusher.Flush()
		}
	}

	return nil
}

// tryNonStreamingRequest attempts a non-streaming request as a fallback when streaming fails
func (h *OpenAIHandler) tryNonStreamingRequest(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Create the OpenAI client
	config := openai.DefaultConfig(h.APIKey)
	config.BaseURL = h.APIBase

	// Use the same transport configuration as streaming
	if h.Debug {
		config.HTTPClient = &http.Client{
			Transport: &utils.DebugTransport{
				Transport: &customHeaderTransport{
					base:     http.DefaultTransport,
					thinking: !h.DisableThinking,
				},
			},
			Timeout: 5 * time.Minute,
		}
	} else {
		config.HTTPClient = &http.Client{
			Transport: &customHeaderTransport{
				base:     http.DefaultTransport,
				thinking: !h.DisableThinking,
			},
			Timeout: 5 * time.Minute,
		}
	}

	client := openai.NewClientWithConfig(config)

	// Create the chat completion request (non-streaming)
	req := openai.ChatCompletionRequest{
		Model: h.ModelName,
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false, // Non-streaming request
	}

	if h.Debug {
		log.Printf("[DEBUG] Making non-streaming request to OpenAI API")
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("[ERROR] Non-streaming request failed: %v", err)
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := resp.Choices[0].Message.Content
	if h.Debug {
		log.Printf("[DEBUG] Non-streaming response received, length: %d bytes", len(content))
	}

	return content, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
