package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
	openai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

// --- Configuration Structures ---
type Config struct {
	Server struct {
		Port       string `yaml:"port"`
		PromptsDir string `yaml:"prompts_dir"`
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

const version = "1.0.0"

var systemPrompt string

func main() {
	// --- Load Configuration ---
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Printf("⚠️  Could not load config.yaml: %v. Using defaults and flags only.", err)
	}

	// --- Define Command-Line Flags ---
	showVersion := flag.Bool("version", false, "Display the version and exit")
	port := flag.String("port", cfg.Server.Port, "Port to run the web server on")
	model := flag.String("model", cfg.Model.Name, "The model to use (for either backend)")
	promptsDir := flag.String("prompts", cfg.Server.PromptsDir, "Directory containing the prompt files")
	backend := flag.String("backend", cfg.Model.Backend, "The AI backend to use ('ollama' or 'openai')")
	apiKey := flag.String("api-key", cfg.OpenAI.APIKey, "OpenAI API key")
	apiBase := flag.String("api-base", cfg.OpenAI.APIBase, "OpenAI-compatible API base URL")
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

	// --- Load the System Prompt at Startup ---
	systemPromptPath := filepath.Join(*promptsDir, "system_prompt.txt")
	promptBytes, err := os.ReadFile(systemPromptPath)
	if err != nil {
		log.Fatalf("❌ Failed to read system prompt file at %s: %v", systemPromptPath, err)
	}
	systemPrompt = string(promptBytes)
	log.Println("✅ System prompt loaded successfully.")

	// --- Setup HTTP Server ---
	// Create a file server for the 'static' directory and handle favicon
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.svg")
	})

	http.HandleFunc("/", handleRequest(*backend, *model, *promptsDir, *apiKey, *apiBase))

	log.Printf("✨ MuseWeb v%s is live at http://localhost:%s", version, *port)
	log.Printf("   (Using backend '%s', model '%s', and prompts from '%s')", *backend, *model, *promptsDir)
	err = http.ListenAndServe(":"+*port, nil)
	if err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

// loadConfig reads the configuration from a YAML file.
func loadConfig(path string) (*Config, error) {
	cfg := &Config{
		// Set default values
		Server: struct {
			Port       string `yaml:"port"`
			PromptsDir string `yaml:"prompts_dir"`
		}{"8080", "./prompts"},
		Model: struct {
			Backend string `yaml:"backend"`
			Name    string `yaml:"name"`
		}{"ollama", "llama3"},
		OpenAI: struct {
			APIKey  string `yaml:"api_key"`
			APIBase string `yaml:"api_base"`
		}{"", "https://api.openai.com/v1"},
	}

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
func handleRequest(backend, modelName, promptsDir, apiKey, apiBase string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// --- Block Asset Requests ---
		path := r.URL.Path
		if strings.Contains(path, ".") && path != "/" {
			log.Printf("🚫 Asset request blocked: %s", path)
			http.NotFound(w, r)
			return
		}

		// --- Determine which User Prompt to Load ---
		promptName := r.URL.Query().Get("prompt")
		if promptName == "" {
			promptName = "home" // Default to home page
		}

		userPromptPath := filepath.Join(promptsDir, filepath.Clean(promptName+".txt"))
		userPromptBytes, err := os.ReadFile(userPromptPath)
		if err != nil {
			log.Printf("⚠️ Could not find prompt file for '%s'. Serving 404.", promptName)
			http.NotFound(w, r)
			return
		}
		userPrompt := string(userPromptBytes)

		log.Printf("🚀 Received request for '%s'. Using backend '%s' with model '%s'.", promptName, backend, modelName)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported!", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// --- Call the selected AI Backend and Stream the Response ---
		if backend == "openai" {
			err = streamOpenAIResponse(w, flusher, modelName, userPrompt, apiKey, apiBase)
		} else {
			err = streamOllamaResponse(w, flusher, modelName, userPrompt)
		}

		if err != nil {
			// Don't write a new error to the response header if one has already been sent.
			// The streaming functions handle their own internal error reporting.
			log.Printf("❌ Stream error: %v", err)
		}
		log.Println("✅ Stream completed.")
	}
}

func streamOllamaResponse(w io.Writer, flusher http.Flusher, modelName, userPrompt string) error {
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
		_, writeErr := io.WriteString(w, res.Message.Content)
		if writeErr != nil {
			log.Printf("🔶 Client disconnected. Aborting stream.")
			return writeErr
		}
		flusher.Flush()
		return nil
	})
}

func streamOpenAIResponse(w io.Writer, flusher http.Flusher, modelName, userPrompt, apiKey, apiBase string) error {
	config := openai.DefaultConfig(apiKey)
	if apiBase != "" {
		config.BaseURL = apiBase
	}
	client := openai.NewClientWithConfig(config)

	req := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Stream: true,
	}

	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		http.Error(w.(http.ResponseWriter), "Failed to start OpenAI stream", http.StatusInternalServerError)
		return err
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil // Stream finished successfully
		}
		if err != nil {
			log.Printf("🔶 OpenAI stream error: %v", err)
			return err
		}

		_, writeErr := io.WriteString(w, response.Choices[0].Delta.Content)
		if writeErr != nil {
			log.Printf("🔶 Client disconnected. Aborting stream.")
			return writeErr
		}
		flusher.Flush()
	}
}
