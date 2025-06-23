package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	openai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

// --- Configuration Structures ---
type Config struct {
	Server struct {
		Address    string `yaml:"address"`
		Port       string `yaml:"port"`
		PromptsDir string `yaml:"prompts_dir"`
		Debug      bool   `yaml:"debug"`
	} `yaml:"server"`
	Model struct {
		Backend string `yaml:"backend"`
		Name    string `yaml:"name"`
	} `yaml:"model"`
	OpenAI struct {
		APIKey  string `yaml:"api_key"`
		APIBase string `yaml:"api_base"`
	} `yaml:"openai"`
}

const version = "1.0.6e"

// codeFenceRE removes markdown code fences like ```html and ```
var codeFenceRE = regexp.MustCompile("```[a-zA-Z]*\\n?|```")

// sanitizeResponse cleans up model output by removing markdown code fences, inline backticks, and empty think tags.
func sanitizeResponse(s string) string {
	// First remove markdown code fences
	cleaned := codeFenceRE.ReplaceAllString(s, "")
	// Remove inline backticks
	cleaned = strings.ReplaceAll(cleaned, "`", "")

	// Remove empty think tags with any amount of whitespace and newlines
	// This handles cases like:
	// <think>
	// </think>
	// or <think></think>
	// or <think>
	// </think>
	cleaned = regexp.MustCompile(`(?i)(?:\s*<think>(?:\s|\n)*</think>\s*)+`).ReplaceAllString(cleaned, "")

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

func main() {
	// --- Load Configuration ---
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Printf("⚠️  Could not load config.yaml: %v. Using defaults and flags only.", err)
	}

	// --- Define Command-Line Flags ---
	showVersion := flag.Bool("version", false, "Display the version and exit")
	host := flag.String("host", cfg.Server.Address, "Interface to bind to (e.g., 127.0.0.1 or 0.0.0.0)")
	port := flag.String("port", cfg.Server.Port, "Port to run the web server on")
	model := flag.String("model", cfg.Model.Name, "The model to use (for either backend)")
	promptsDir := flag.String("prompts", cfg.Server.PromptsDir, "Directory containing the prompt files")
	backend := flag.String("backend", cfg.Model.Backend, "The AI backend to use ('ollama' or 'openai')")
	apiKey := flag.String("api-key", cfg.OpenAI.APIKey, "OpenAI API key")
	apiBase := flag.String("api-base", cfg.OpenAI.APIBase, "OpenAI-compatible API base URL")
	debug := flag.Bool("debug", cfg.Server.Debug, "Enable debug mode to print request JSON")
	flag.Parse()

	if *showVersion {
		fmt.Printf("MuseWeb v%s\n", version)
		os.Exit(0)
	}

	// --- Final Configuration ---
	// If the api-key flag is still empty, try the environment variable as a last resort.
	if *apiKey == "" {
		*apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// --- Validate OpenAI Config ---
	if *backend == "openai" && *apiKey == "" {
		log.Fatalf("❌ For the 'openai' backend, the API key must be provided via the -api-key flag, the config.yaml file, or the OPENAI_API_KEY environment variable.")
	}

	// --- Setup HTTP Server ---
	appHandler := http.HandlerFunc(handleRequest(*backend, *model, *promptsDir, *apiKey, *apiBase, *debug))
	fs := http.FileServer(http.Dir("public"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve static files if the path contains a dot (file extension)
		if strings.Contains(r.URL.Path, ".") {
			fs.ServeHTTP(w, r)
			return
		}
		// Otherwise, handle as a prompt request
		appHandler.ServeHTTP(w, r)
	})

	displayHost := *host
	if *host == "0.0.0.0" {
		displayHost = "localhost"
	}
	log.Printf("✨ MuseWeb v%s is live at http://%s:%s", version, displayHost, *port)
	log.Printf("   (Using backend '%s', model '%s', and prompts from '%s')", *backend, *model, *promptsDir)

	listenAddr := *host
	if listenAddr == "0.0.0.0" {
		listenAddr = ""
	}
	err = http.ListenAndServe(listenAddr+":"+*port, nil)
	if err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

// loadConfig reads the configuration from a YAML file.
func loadConfig(path string) (*Config, error) {
	// Default configuration
	cfg := &Config{}
	cfg.Server.Address = "127.0.0.1"
	cfg.Server.Port = "8000"
	cfg.Server.PromptsDir = "./prompts"
	cfg.Server.Debug = false
	cfg.Model.Backend = "ollama"
	cfg.Model.Name = "llama3"
	cfg.OpenAI.APIBase = "https://api.openai.com/v1"

	file, err := os.ReadFile(path)
	if err != nil {
		return cfg, err // Return defaults if file doesn't exist
	}

	err = yaml.Unmarshal(file, cfg)
	if err != nil {
		return nil, err // Return error for parsing issues
	}

	return cfg, nil
}

// handleRequest returns a handler function that processes incoming requests.
func handleRequest(backend, modelName, promptsDir, apiKey, apiBase string, debug bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// --- Load System Prompt ---
		systemPromptPath := filepath.Join(promptsDir, "system_prompt.txt")
		systemPromptBytes, err := os.ReadFile(systemPromptPath)
		if err != nil {
			log.Printf("❌ Failed to read system prompt file at %s: %v", systemPromptPath, err)
			http.Error(w, "Could not load system prompt", http.StatusInternalServerError)
			return
		}
		systemPrompt := string(systemPromptBytes)

		// --- Append Layout Prompt if it exists ---
		layoutPromptPath := filepath.Join(promptsDir, "layout.txt")
		if layoutBytes, err := os.ReadFile(layoutPromptPath); err == nil {
			systemPrompt += "\n" + string(layoutBytes)
			log.Printf("🎨 Loaded layout prompt from %s", layoutPromptPath)
		} else if os.IsNotExist(err) {
			// This is not an error, just for information.
			log.Printf("ℹ️  No layout prompt found at %s. Skipping.", layoutPromptPath)
		} else {
			// Log an error if the file exists but couldn't be read for some other reason.
			log.Printf("⚠️ Could not read layout prompt file at %s: %v", layoutPromptPath, err)
		}

		// --- Determine which User Prompt to Load ---
		// Extract path and remove leading slash
		path := strings.TrimPrefix(r.URL.Path, "/")

		promptName := path
		if promptName == "" {
			promptName = "home" // Default to home page
		}

		userPromptPath := filepath.Join(promptsDir, filepath.Clean(promptName+".txt"))
		log.Printf("📄 Attempting to load prompt file: %s", userPromptPath)
		userPromptBytes, err := os.ReadFile(userPromptPath)
		if err != nil {
			log.Printf("⚠️ Could not find prompt file for '%s' at path '%s'. Serving 404.", promptName, userPromptPath)
			http.NotFound(w, r)
			return
		}
		userPrompt := string(userPromptBytes)

		log.Printf("🚀 Received request for '%s' (file: %s). Using backend '%s' with model '%s'.", promptName, userPromptPath, backend, modelName)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported!", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// --- Print debug information if requested ---
		if debug {
			printRequestDebugInfo(backend, modelName, systemPrompt, userPrompt)
		}

		// --- Call the selected AI Backend and Stream the Response ---
		if backend == "openai" {
			err = streamOpenAIResponse(w, flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase)
		} else {
			err = streamOllamaResponse(w, flusher, modelName, systemPrompt, userPrompt)
		}

		if err != nil {
			// Don't write a new error to the response header if one has already been sent.
			// The streaming functions handle their own internal error reporting.
			log.Printf("❌ Stream error: %v", err)
		}
		log.Println("✅ Stream completed.")
	}
}

func streamOllamaResponse(w io.Writer, flusher http.Flusher, modelName, systemPrompt, userPrompt string) error {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		http.Error(w.(http.ResponseWriter), "Failed to create Ollama client", http.StatusInternalServerError)
		return err
	}

	req := &api.ChatRequest{
		Model: modelName,
		Messages: []api.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: &[]bool{true}[0],
	}

	return client.Chat(context.Background(), req, func(res api.ChatResponse) error {
		_, writeErr := io.WriteString(w, sanitizeResponse(res.Message.Content))
		if writeErr != nil {
			log.Printf("🔶 Client disconnected. Aborting stream.")
			return writeErr
		}
		flusher.Flush()
		return nil
	})
}

// streamClaudeResponse handles streaming responses from Claude models with the correct API structure
func streamClaudeResponse(ctx context.Context, w io.Writer, flusher http.Flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase string) error {
	// Determine the API endpoint
	baseURL := "https://api.anthropic.com"
	if apiBase != "" {
		baseURL = apiBase
	}

	// Create the request body with the correct structure for Claude models
	// The system prompt is a separate top-level parameter
	requestBody := map[string]interface{}{
		"model":  modelName,
		"system": systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
		"stream":     true,
		"max_tokens": 4096,
	}

	// Marshal the request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("❌ Failed to marshal Claude request: %v", err)
		return err
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Failed to create Claude request: %v", err)
		return err
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Create an HTTP client with appropriate timeouts
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 2 * time.Minute,
			TLSHandshakeTimeout:   30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Failed to send Claude request: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Claude API error: %s - %s", resp.Status, string(body))
		return fmt.Errorf("Claude API error: %s", resp.Status)
	}

	// Track if we've received any content
	contentReceived := false

	// Process the streaming response
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract the JSON data
		jsonData := strings.TrimPrefix(line, "data: ")

		// Check for the [DONE] message
		if jsonData == "[DONE]" {
			break
		}

		// Parse the JSON
		var streamResponse map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &streamResponse); err != nil {
			log.Printf("⚠️ Failed to parse Claude stream response: %v", err)
			continue
		}

		// Extract the content delta
		type Delta struct {
			Type    string `json:"type"`
			Content string `json:"text"`
		}

		// Navigate the response structure to find the content
		if contentDelta, ok := streamResponse["delta"]; ok {
			if deltaMap, ok := contentDelta.(map[string]interface{}); ok {
				if text, ok := deltaMap["text"].(string); ok && text != "" {
					contentReceived = true

					// Process and write the content
					sanitized := sanitizeResponse(text)
					_, writeErr := io.WriteString(w, sanitized)
					if writeErr != nil {
						log.Printf("🔶 Client disconnected. Aborting stream.")
						return writeErr
					}
					flusher.Flush()
				}
			}
		}
	}

	if !contentReceived {
		log.Printf("⚠️ Claude stream ended without receiving any content")
	}

	if err := scanner.Err(); err != nil {
		log.Printf("🔶 Claude stream error: %v", err)

		// If we've already received some content, don't return an error to the user
		if contentReceived {
			log.Printf("⚠️ Stream error after partial content. Ending stream gracefully.")
			return nil
		}
		return err
	}

	return nil
}

// printRequestDebugInfo prints the request information in JSON format for debugging
func printRequestDebugInfo(backend, modelName, systemPrompt, userPrompt string) {
	type DebugMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type DebugRequest struct {
		Backend  string         `json:"backend"`
		Model    string         `json:"model"`
		System   string         `json:"system,omitempty"`
		Messages []DebugMessage `json:"messages"`
	}

	// Create a debug request object
	debugReq := DebugRequest{
		Backend: backend,
		Model:   modelName,
	}

	// Check if this is a Claude model
	isClaudeModel := strings.Contains(strings.ToLower(modelName), "claude")

	// Use the same message format that will be sent to the API
	if isClaudeModel {
		// For Claude models, set system prompt as a separate top-level parameter
		// and put only the user prompt in the messages array
		debugReq.System = systemPrompt
		debugReq.Messages = []DebugMessage{
			{Role: "user", Content: userPrompt},
		}
	} else {
		// For other models, use separate system and user messages
		debugReq.Messages = []DebugMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	}

	jsonBytes, err := json.MarshalIndent(debugReq, "", "  ")
	if err != nil {
		log.Printf("⚠️ Error marshaling debug info: %v", err)
		return
	}

	log.Printf("🔍 DEBUG REQUEST JSON:\n%s\n", string(jsonBytes))
}

func streamOpenAIResponse(w io.Writer, flusher http.Flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase string) error {
	// Create a context with a generous timeout for large responses
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check if the model name contains "claude" (case insensitive)
	isClaudeModel := strings.Contains(strings.ToLower(modelName), "claude")

	// For Claude models, we need to use a custom HTTP client to properly structure the request
	if isClaudeModel {
		return streamClaudeResponse(ctx, w, flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase)
	}

	// For non-Claude models, use the standard OpenAI client
	config := openai.DefaultConfig(apiKey)
	if apiBase != "" {
		config.BaseURL = apiBase
	}

	// Set higher timeout values for HTTP client
	config.HTTPClient = &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 2 * time.Minute,
			TLSHandshakeTimeout:   30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	client := openai.NewClientWithConfig(config)

	// Create the request with standard OpenAI message structure
	req := openai.ChatCompletionRequest{
		Model:     modelName,
		Stream:    true,
		MaxTokens: 4096,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		http.Error(w.(http.ResponseWriter), "Failed to start OpenAI stream", http.StatusInternalServerError)
		return err
	}
	defer stream.Close()

	// Track if we've received any content
	contentReceived := false

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if !contentReceived {
				log.Printf("⚠️ Stream ended without receiving any content")
			}
			return nil // Stream finished successfully
		}
		if err != nil {
			log.Printf("🔶 OpenAI stream error: %v", err)

			// If we've already received some content, don't return an error to the user
			// This helps with partial responses being better than no response
			if contentReceived {
				log.Printf("⚠️ Stream error after partial content. Ending stream gracefully.")
				return nil
			}
			return err
		}

		// If we have content in this chunk
		if response.Choices[0].Delta.Content != "" {
			contentReceived = true

			// Process and write the content
			sanitized := sanitizeResponse(response.Choices[0].Delta.Content)
			_, writeErr := io.WriteString(w, sanitized)
			if writeErr != nil {
				log.Printf("🔶 Client disconnected. Aborting stream.")
				return writeErr
			}
			flusher.Flush()
		}
	}
}
