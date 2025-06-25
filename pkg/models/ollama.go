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

	// Define a callback function to handle streaming responses
	callbackFn := func(response api.ChatResponse) error {
		if response.Message.Content != "" {
			fullResponse.WriteString(response.Message.Content)
		}
		return nil
	}

	// Call the Chat method with the callback function
	err := client.Chat(ctx, &req, callbackFn)
	if err != nil {
		return fmt.Errorf("failed to start Ollama chat: %w", err)
	}

	// --- DEBUG: Print full raw provider response before any processing ---
	log.Printf("[PROVIDER RAW RESPONSE] (Ollama)\n%s", fullResponse.String())

	// Process the full response
	finalOutput := utils.ProcessModelOutput(fullResponse.String(), h.ModelName, !h.DisableThinking)

	// Write the final, clean output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	return nil
}
