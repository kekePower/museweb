package models

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/kekePower/museweb/pkg/utils"
)

// OllamaHandler implements the ModelHandler interface for Ollama
type OllamaHandler struct {
	ModelName       string
	APIKey          string
	APIBase         string
	DisableThinking bool
	Debug           bool
}

// Streaming state tracking
var (
	ollamaStreamingStarted bool  // Have we started streaming to client?
	ollamaLastSentLength   int   // How much have we sent so far?
)

// processStreamingContent implements smart streaming:
// 1. Buffer until we find HTML start (<!DOCTYPE, <html>)
// 2. Stream content in real-time to client
// 3. Stop streaming after </html>, discard everything after
func processOllamaStreamingContent(newContent string, pendingBuffer *strings.Builder) string {
	// Add new content to pending buffer
	pendingBuffer.WriteString(newContent)
	bufferContent := pendingBuffer.String()
	
	// Phase 1: Look for HTML start if we haven't started streaming yet
	if !ollamaStreamingStarted {
		// Look for HTML document start patterns
		htmlStartPos := -1
		if strings.Contains(bufferContent, "<!DOCTYPE") {
			htmlStartPos = strings.Index(bufferContent, "<!DOCTYPE")
		} else if strings.Contains(bufferContent, "<html") {
			htmlStartPos = strings.Index(bufferContent, "<html")
		}
		
		if htmlStartPos != -1 {
			// Found HTML start! Begin streaming from this point
			ollamaStreamingStarted = true
			ollamaLastSentLength = htmlStartPos
			
			// Send everything from HTML start to current buffer end
			contentToSend := bufferContent[htmlStartPos:]
			ollamaLastSentLength = len(bufferContent)
			return contentToSend
		}
		
		// No HTML start found yet, keep buffering
		return ""
	}
	
	// Phase 2: We're streaming - check if we've reached HTML end
	htmlEndPos := strings.Index(strings.ToLower(bufferContent), "</html>")
	
	if htmlEndPos == -1 {
		// No </html> yet - continue streaming new content
		if len(bufferContent) > ollamaLastSentLength {
			newPortion := bufferContent[ollamaLastSentLength:]
			ollamaLastSentLength = len(bufferContent)
			return newPortion
		}
		return ""
		
	} else {
		// Found </html>! Send final portion and stop streaming
		htmlEndFull := htmlEndPos + len("</html>")
		
		// Send any remaining content up to and including </html>
		var finalContent string
		if htmlEndFull > ollamaLastSentLength {
			finalContent = bufferContent[ollamaLastSentLength:htmlEndFull]
		}
		
		// Reset state for next request
		pendingBuffer.Reset()
		ollamaStreamingStarted = false
		ollamaLastSentLength = 0
		
		// Everything after </html> goes to /dev/null (discarded)
		return finalContent
	}
}

// StreamResponse streams the response from the Ollama model
func (h *OllamaHandler) StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	ctx := context.Background()

	// Determine base URL (config api_base or fallback)
	endpoint := h.APIBase
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	baseURL, _ := url.Parse(endpoint)

	// Prepare HTTP client, adding Authorization header if API key supplied and debug transport if debug enabled
	httpClient := http.DefaultClient
	if h.APIKey != "" {
		if h.Debug {
			// Use debug transport when debug mode is enabled
			httpClient = &http.Client{
				Transport: &utils.DebugTransport{
					Transport: &authTransport{
						base:   http.DefaultTransport,
						apiKey: h.APIKey,
					},
				},
				Timeout: 5 * time.Minute,
			}
			log.Printf("[DEBUG] HTTP debugging enabled for Ollama client")
		} else {
			// Use standard transport without debug logging
			httpClient = &http.Client{
				Transport: &authTransport{
					base:   http.DefaultTransport,
					apiKey: h.APIKey,
				},
				Timeout: 5 * time.Minute,
			}
		}
	} else if h.Debug {
		// No API key but debug is enabled
		httpClient = &http.Client{
			Transport: &utils.DebugTransport{
				Transport: http.DefaultTransport,
			},
			Timeout: 5 * time.Minute,
		}
		log.Printf("[DEBUG] HTTP debugging enabled for Ollama client")
	}
	client := api.NewClient(baseURL, httpClient)

	streamOption := true
	req := api.ChatRequest{
		Model: h.ModelName,
		Messages: []api.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: &streamOption,
	}

	var fullResponse strings.Builder
	var pendingBuffer strings.Builder

	// Define a callback function to handle streaming responses
	callbackFn := func(response api.ChatResponse) error {
		if response.Message.Content != "" {
			content := response.Message.Content
			fullResponse.WriteString(content)
			
			// Process content for real-time streaming using the same logic as OpenAI custom
			processedContent := processOllamaStreamingContent(content, &pendingBuffer)
			

			
			// Send processed content to client immediately
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
		return nil
	}

	// Call the Chat method with the callback function
	err := client.Chat(ctx, &req, callbackFn)
	if err != nil {
		return fmt.Errorf("failed to start Ollama chat: %w", err)
	}

	// --- DEBUG: Print full raw provider response before any processing ---
	if h.Debug {
		log.Printf("[PROVIDER RAW RESPONSE] (Ollama)\n%s", fullResponse.String())
	}

	// Flush any remaining content in the pending buffer at the end of stream
	if pendingBuffer.Len() > 0 {
		// Apply final cleanup to any remaining pending content
		finalPending := utils.CleanupCodeFences(pendingBuffer.String())
		
		// Additional end-of-stream cleanup for any remaining backticks
		finalPending = strings.TrimSpace(finalPending)
		if strings.HasSuffix(finalPending, "```") {
			finalPending = strings.TrimSuffix(finalPending, "```")
			finalPending = strings.TrimSpace(finalPending)
		}
		
		if finalPending != "" {
			_, err := io.WriteString(w, finalPending)
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
	return nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
