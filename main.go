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
		// EnableThinking enables the thinking tag for DeepSeek and r1-1776 models
		EnableThinking bool `yaml:"enable_thinking"`
	} `yaml:"model"`
	OpenAI struct {
		APIKey  string `yaml:"api_key"`
		APIBase string `yaml:"api_base"`
	} `yaml:"openai"`
}

const version = "1.0.8"

// codeFenceRE removes markdown code fences like ```html and ```
var codeFenceRE = regexp.MustCompile("```[a-zA-Z]*\\n?|```")

// sanitizeResponse cleans up model output by removing markdown code fences, inline backticks, and think tags with their content.
// This function serves as the final safety net in our multi-layered approach to handling model outputs.
func sanitizeResponse(s string) string {
	// First remove markdown code fences
	cleaned := codeFenceRE.ReplaceAllString(s, "")
	// Remove inline backticks
	cleaned = strings.ReplaceAll(cleaned, "`", "")

	// Extract thinking content first (for logging or discarding)
	thinking := extractThinking(cleaned)
	if thinking != "" {
		// Send thinking to /dev/null (or log it for debugging)
		fmt.Fprintf(io.Discard, "Model thinking: %s\n", thinking)

		// Uncomment the line below if you want to see the thinking in the logs
		// log.Printf("Model thinking from sanitize: %s", thinking)
	}

	// Remove think tags and their content (for DeepSeek models including r-1776)
	// This regex matches <think> tag, any content inside (including newlines), and the closing </think> tag
	cleaned = regexp.MustCompile(`(?i)<think>(?s:.*?)</think>`).ReplaceAllString(cleaned, "")

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
	promptsDir := flag.String("prompts", cfg.Server.PromptsDir, "Directory containing prompt files")
	backend := flag.String("backend", cfg.Model.Backend, "AI backend to use (ollama or openai)")
	model := flag.String("model", cfg.Model.Name, "Model name to use")
	apiKey := flag.String("api-key", cfg.OpenAI.APIKey, "API key for OpenAI-compatible APIs")
	apiBase := flag.String("api-base", cfg.OpenAI.APIBase, "Base URL for OpenAI-compatible APIs")
	debug := flag.Bool("debug", cfg.Server.Debug, "Enable debug mode")
	enableThinking := flag.Bool("enable-thinking", cfg.Model.EnableThinking, "Enable thinking tag for DeepSeek and r1-1776 models")
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
	appHandler := http.HandlerFunc(handleRequest(*backend, *model, *promptsDir, *apiKey, *apiBase, *debug, *enableThinking))
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
	if *enableThinking && isThinkingEnabledModel(*model) {
		log.Printf("   🧠 Thinking tag enabled for %s model", *model)
	}

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
func handleRequest(backend, modelName, promptsDir, apiKey, apiBase string, debug bool, enableThinking bool) http.HandlerFunc {
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
			printRequestDebugInfo(backend, modelName, systemPrompt, userPrompt, enableThinking)
		}

		// --- Create a buffer to capture the model output ---
		var outputBuffer bytes.Buffer

		// --- Call the selected AI Backend and Stream the Response to the buffer ---
		if backend == "openai" {
			err = streamOpenAIResponse(&outputBuffer, flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase, enableThinking)
		} else {
			err = streamOllamaResponse(&outputBuffer, flusher, modelName, systemPrompt, userPrompt, enableThinking)
		}

		if err != nil {
			// Don't write a new error to the response header if one has already been sent.
			// The streaming functions handle their own internal error reporting.
			log.Printf("❌ Stream error: %v", err)
			return
		}

		// --- Get the final output from the buffer ---
		output := outputBuffer.String()
		log.Println("✅ Model response received, processing output...")

		// --- Check if the output is JSON and extract the HTML content ---
		if strings.HasPrefix(strings.TrimSpace(output), "{") {
			var response struct {
				Answer string `json:"answer"`
			}

			if err := json.Unmarshal([]byte(output), &response); err == nil && response.Answer != "" {
				log.Println("✅ Successfully extracted HTML from JSON response")
				// Write the HTML content from the answer field
				_, err = io.WriteString(w, response.Answer)
				if err != nil {
					log.Printf("❌ Error writing response to client: %v", err)
				}
				return
			} else if err != nil {
				log.Printf("⚠️ Could not parse JSON response: %v", err)
			}
		}

		// --- Fallback: write the output directly if it's not valid JSON or doesn't have an answer field ---
		log.Println("✅ Using direct output (no JSON found or couldn't parse)")
		_, err = io.WriteString(w, output)
		if err != nil {
			log.Printf("❌ Error writing response to client: %v", err)
		}
		log.Println("✅ Response sent to client.")
	}
}

func streamOllamaResponse(w io.Writer, flusher http.Flusher, modelName, systemPrompt, userPrompt string, enableThinking bool) error {
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
		Stream: &[]bool{true}[0], // We still stream from the model
	}

	// Add thinking tag for DeepSeek and r1-1776 models if enabled
	if isThinkingEnabledModel(modelName) {
		req.Options = map[string]interface{}{
			"thinking": !enableThinking, // Send thinking: false when enableThinking is true
		}
		log.Printf("🧠 Setting thinking: %v for Ollama model %s", !enableThinking, modelName)
	}

	// Use a strings.Builder to buffer the full response
	var fullResponse strings.Builder

	err = client.Chat(context.Background(), req, func(res api.ChatResponse) error {
		// Append each chunk to the builder
		_, writeErr := fullResponse.WriteString(res.Message.Content)
		if writeErr != nil {
			log.Printf("Error building string: %v", writeErr)
			return writeErr
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Now that we have the full response, process it correctly
	finalOutput := processModelOutput(fullResponse.String(), modelName, enableThinking)

	// Write the final, clean output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	return nil
}

func printRequestDebugInfo(backend, modelName, systemPrompt, userPrompt string, enableThinking bool) {
	type DebugMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type DebugRequest struct {
		Backend  string         `json:"backend"`
		Model    string         `json:"model"`
		System   string         `json:"system,omitempty"`
		Messages []DebugMessage `json:"messages"`
		Thinking bool           `json:"thinking,omitempty"`
	}

	// Create a debug request object
	debugReq := DebugRequest{
		Backend: backend,
		Model:   modelName,
	}

	// Check if thinking tag should be enabled
	if isThinkingEnabledModel(modelName) && enableThinking {
		debugReq.Thinking = true
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

// ModelResponse represents a structured response from the model with separate thinking and answer fields
type ModelResponse struct {
	Thinking string `json:"thinking"`
	Answer   string `json:"answer"`
}

// customHeaderTransport is a custom http.RoundTripper that adds headers to requests
type customHeaderTransport struct {
	base     http.RoundTripper
	thinking bool
}

// RoundTrip implements the http.RoundTripper interface
func (t *customHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add the thinking header if enabled
	if t.thinking {
		req.Header.Add("X-Thinking", "true")
	}

	// Use the base transport or default if not provided
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(req)
}

// isThinkingEnabledModel checks if the model is one that supports the thinking tag
func isThinkingEnabledModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	// Check if it's a DeepSeek model or r1-1776 from Perplexity
	return strings.Contains(modelName, "deepseek") || strings.Contains(modelName, "r1-1776")
}

// shouldSanitize determines if we should apply internal sanitization based on model and thinking settings
// If thinking=true is sent to server, server handles sanitization so we should skip it
// If thinking=false is sent to server, we handle sanitization internally
func shouldSanitize(modelName string, enableThinking bool) bool {
	// For models that don't support thinking tag, always sanitize
	if !isThinkingEnabledModel(modelName) {
		return true
	}

	// For thinking-enabled models, sanitize only if we're handling it (thinking=false sent to server)
	return enableThinking
}

// extractThinking attempts to extract thinking/reasoning content from model output
// This can be from <think> tags or from JSON structure
func extractThinking(output string) string {
	// Try to extract thinking from <think> tags
	thinkRegex := regexp.MustCompile(`(?i)<think>(?s:(.*?))</think>`)
	matches := thinkRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try to extract thinking from JSON
	jsonRegex := regexp.MustCompile(`(?s)\s*\{.*\}\s*`)
	jsonMatch := jsonRegex.FindString(output)
	if jsonMatch != "" {
		var response ModelResponse
		err := json.Unmarshal([]byte(jsonMatch), &response)
		if err == nil && response.Thinking != "" {
			return response.Thinking
		}
	}

	return ""
}

// processModelOutput attempts to parse structured data and conditionally sanitizes the result
// based on model and thinking settings
func processModelOutput(rawOutput string, modelName string, enableThinking bool) string {
	// First, let's try to find the primary payload, which can be JSON or HTML

	// Attempt to find a JSON object anywhere in the output
	jsonRegex := regexp.MustCompile(`(?s)\s*(\{.*\})\s*`)
	jsonMatch := jsonRegex.FindStringSubmatch(rawOutput)

	var payload string
	if len(jsonMatch) > 1 {
		// A JSON object was found. Assume this is the intended structured output
		var response ModelResponse
		err := json.Unmarshal([]byte(jsonMatch[1]), &response)

		if err == nil && response.Answer != "" {
			log.Println("Successfully parsed structured JSON response.")
			if response.Thinking != "" {
				// Send thinking to /dev/null (or log it for debugging)
				fmt.Fprintf(io.Discard, "Model thinking (from JSON): %s\n", response.Thinking)

				// Uncomment the line below if you want to see the thinking in the logs
				// log.Printf("Model thinking (from JSON): %s", response.Thinking)
			}
			payload = response.Answer
		} else {
			// Found something that looked like JSON but failed to parse
			// This is an ambiguous case. We'll treat the whole raw output
			// as the payload and let the sanitizer clean it up
			log.Println("Found JSON-like content but couldn't parse it. Falling back to raw output.")
			payload = rawOutput
		}
	} else {
		// No JSON found. Assume the entire output is the intended payload
		payload = rawOutput
	}

	// Conditionally apply sanitization based on model and thinking settings
	var finalResult string
	if shouldSanitize(modelName, enableThinking) {
		// Apply internal sanitization
		log.Printf("🧹 Applying internal sanitization for model %s (thinking enabled: %v)", modelName, enableThinking)
		finalResult = sanitizeResponse(payload)
	} else {
		// Skip sanitization, server is handling it
		log.Printf("⏩ Skipping internal sanitization for model %s (server handling it)", modelName)
		finalResult = payload
	}

	// One final trim to remove leading/trailing whitespace that might result from stripping tags
	return strings.TrimSpace(finalResult)
}

func streamOpenAIResponse(w io.Writer, flusher http.Flusher, modelName, systemPrompt, userPrompt, apiKey, apiBase string, enableThinking bool) error {
	// Create a context with a generous timeout for large responses
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create OpenAI client
	config := openai.DefaultConfig(apiKey)
	if apiBase != "" {
		config.BaseURL = apiBase
	}
	config.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	client := openai.NewClientWithConfig(config)

	// Create chat completion request
	req := openai.ChatCompletionRequest{
		Model:     modelName,
		Stream:    true,
		MaxTokens: 6144, // Increased to handle larger responses
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	}

	// Add the thinking tag for DeepSeek and r1-1776 models if enabled
	if isThinkingEnabledModel(modelName) {
		// Add the thinking parameter directly to the request JSON
		// We need to use a custom JSON marshaling approach since the go-openai library
		// doesn't support arbitrary fields

		// Log the original system prompt before any modifications
		log.Printf("🔍 Original system prompt for %s (before custom request): %s", modelName, systemPrompt[:100]+"...")

		// First, convert our request to a map
		reqMap := map[string]interface{}{
			"model":      modelName,
			"stream":     true,
			"max_tokens": 6144,
			"messages":   req.Messages,
			"thinking":   !enableThinking, // Send thinking: false when enableThinking is true
			// Force direct_response: false to prevent system prompt modification
			"direct_response": false,
		}

		// Create a custom request
		jsonData, err := json.Marshal(reqMap)
		if err != nil {
			return fmt.Errorf("error marshaling request with thinking tag: %w", err)
		}

		// Log the outgoing JSON payload for debugging
		log.Printf("🔍 Outgoing JSON payload for %s:\n%s", modelName, string(jsonData))

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
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		// Add a custom header to prevent middleware from modifying our request
		httpReq.Header.Set("X-Preserve-System-Prompt", "true")

		// Log the headers being sent
		log.Printf("🔍 Request headers for %s: %v", modelName, httpReq.Header)

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
		log.Printf("🧠 Setting thinking: %v for OpenAI model %s", !enableThinking, modelName)

		// Now that the stream is complete, process the full response
		finalOutput := processModelOutput(fullResponse.String(), modelName, enableThinking)

		// Write the final, clean output to the client
		_, writeErr := io.WriteString(w, finalOutput)
		if writeErr != nil {
			log.Printf("Client disconnected before final write.")
			return writeErr
		}
		flusher.Flush()
		return nil
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		http.Error(w.(http.ResponseWriter), "Failed to start OpenAI stream", http.StatusInternalServerError)
		return err
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
	finalOutput := processModelOutput(fullResponse.String(), modelName, enableThinking)

	// Write the final, clean output to the client
	_, writeErr := io.WriteString(w, finalOutput)
	if writeErr != nil {
		log.Printf("Client disconnected before final write.")
		return writeErr
	}
	flusher.Flush()
	return nil
}
