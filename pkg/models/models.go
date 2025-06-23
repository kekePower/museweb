package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	openai "github.com/sashabaranov/go-openai"

	"github.com/kekePower/museweb/pkg/utils"
)

// ModelHandler is an interface for different AI model backends
type ModelHandler interface {
	StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error
}

// OllamaHandler implements the ModelHandler interface for Ollama
type OllamaHandler struct {
	ModelName      string
	EnableThinking bool
}

// OpenAIHandler implements the ModelHandler interface for OpenAI-compatible APIs
type OpenAIHandler struct {
	ModelName      string
	APIKey         string
	APIBase        string
	EnableThinking bool
}

// NewModelHandler creates a new model handler based on the backend type
func NewModelHandler(backend, modelName, apiKey, apiBase string, enableThinking bool) ModelHandler {
	switch strings.ToLower(backend) {
	case "openai":
		return &OpenAIHandler{
			ModelName:      modelName,
			APIKey:         apiKey,
			APIBase:        apiBase,
			EnableThinking: enableThinking,
		}
	default:
		return &OllamaHandler{
			ModelName:      modelName,
			EnableThinking: enableThinking,
		}
	}
}

// StreamResponse streams the response from the Ollama model
func (h *OllamaHandler) StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	ctx := context.Background()

	// Create Ollama client with default endpoint
	baseURL, _ := url.Parse("http://localhost:11434")
	client := api.NewClient(baseURL, http.DefaultClient)

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

	// Process the full response
	finalOutput := utils.ProcessModelOutput(fullResponse.String(), h.ModelName, h.EnableThinking)

	// Write the final, clean output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	return nil
}

// customHeaderTransport is a custom http.RoundTripper that adds headers to requests
type customHeaderTransport struct {
	base     http.RoundTripper
	thinking bool
}

// RoundTrip implements the http.RoundTripper interface
func (t *customHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add custom headers to the request
	if t.thinking {
		req.Header.Set("X-Thinking-Enabled", "true")
	}
	// Use the base transport to perform the actual request
	return t.base.RoundTrip(req)
}

// StreamResponse streams the response from the OpenAI model
func (h *OpenAIHandler) StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	ctx := context.Background()

	// Configure OpenAI client
	config := openai.DefaultConfig(h.APIKey)
	if h.APIBase != "" {
		config.BaseURL = h.APIBase
	}

	// Add custom transport for thinking header
	baseTransport := http.DefaultTransport
	config.HTTPClient = &http.Client{
		Transport: &customHeaderTransport{
			base:     baseTransport,
			thinking: h.EnableThinking,
		},
		Timeout: 5 * time.Minute,
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

	// For models that support thinking tags, use the custom API approach
	if utils.IsThinkingEnabledModel(h.ModelName) {
		// Create a custom request with the thinking parameter
		reqMap := map[string]interface{}{
			"model":      h.ModelName,
			"stream":     true,
			"max_tokens": 6144,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
			"thinking":        !h.EnableThinking, // Send thinking: false when enableThinking is true
			"direct_response": false,
		}

		// Create a custom request
		jsonData, err := json.Marshal(reqMap)
		if err != nil {
			return fmt.Errorf("error marshaling request with thinking tag: %w", err)
		}

		// Log the outgoing JSON payload for debugging
		log.Printf("🔍 Outgoing JSON payload for %s:\n%s", h.ModelName, string(jsonData))

		// Create HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			"POST",
			config.BaseURL+"/chat/completions",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			return fmt.Errorf("error creating request with thinking tag: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+h.APIKey)
		// Add a custom header to prevent middleware from modifying our request
		httpReq.Header.Set("X-Preserve-System-Prompt", "true")

		// Log the headers being sent
		log.Printf("🔍 Request headers for %s: %v", h.ModelName, httpReq.Header)

		// Create HTTP client
		httpClient := &http.Client{Timeout: 5 * time.Minute}

		// Send request
		httpResp, err := httpClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("error sending request with thinking tag: %w", err)
		}
		defer httpResp.Body.Close()

		// Check response status
		if httpResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(httpResp.Body)
			return fmt.Errorf("error from OpenAI API: %s - %s", httpResp.Status, string(body))
		}

		// Process the streaming response
		var fullResponse strings.Builder
		reader := bufio.NewReader(httpResp.Body)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("error reading stream: %w", err)
			}

			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var streamResp openai.ChatCompletionStreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					continue
				}

				if len(streamResp.Choices) > 0 && streamResp.Choices[0].Delta.Content != "" {
					fullResponse.WriteString(streamResp.Choices[0].Delta.Content)
				}
			}
		}

		// Log that we're setting the thinking tag
		log.Printf("🧠 Setting thinking: %v for OpenAI model %s", !h.EnableThinking, h.ModelName)

		// Now that the stream is complete, process the full response
		finalOutput := utils.ProcessModelOutput(fullResponse.String(), h.ModelName, h.EnableThinking)

		// Write the final, clean output to the client
		_, writeErr := io.WriteString(w, finalOutput)
		if writeErr != nil {
			log.Printf("Client disconnected before final write.")
			return writeErr
		}
		flusher.Flush()
		return nil
	}

	// Standard OpenAI API approach for models without thinking tag support
	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start OpenAI stream: %w", err)
	}
	defer stream.Close()

	// Use a strings.Builder to buffer the full response
	var fullResponse strings.Builder

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break // Stream finished
		}
		if err != nil {
			log.Printf("OpenAI stream error: %v", err)
			return err
		}

		if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
			// Append each chunk to the builder
			fullResponse.WriteString(response.Choices[0].Delta.Content)
		}
	}

	// Now that the stream is complete, process the full response
	finalOutput := utils.ProcessModelOutput(fullResponse.String(), h.ModelName, h.EnableThinking)

	// Write the final, clean output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	return nil
}
