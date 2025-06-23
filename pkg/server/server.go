package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kekePower/museweb/pkg/models"
)

// DebugMessage represents a message in the debug output
type DebugMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DebugRequest represents the full request for debug output
type DebugRequest struct {
	Backend  string         `json:"backend"`
	Model    string         `json:"model"`
	System   string         `json:"system,omitempty"`
	Messages []DebugMessage `json:"messages"`
	Thinking bool           `json:"thinking,omitempty"`
}

// PrintRequestDebugInfo logs debug information about the request
func PrintRequestDebugInfo(backend, modelName, systemPrompt, userPrompt string, enableThinking bool) {
	// Create a debug request object for structured logging
	debugReq := DebugRequest{
		Backend: backend,
		Model:   modelName,
		System:  systemPrompt,
		Messages: []DebugMessage{
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Thinking: enableThinking,
	}

	// Log the debug information
	log.Printf("🔍 Debug Info:\n")
	log.Printf("🔍 Backend: %s\n", debugReq.Backend)
	log.Printf("🔍 Model: %s\n", debugReq.Model)
	log.Printf("🔍 Thinking Enabled: %v\n", debugReq.Thinking)
	log.Printf("🔍 System Prompt: %s\n", debugReq.System)
	log.Printf("🔍 User Prompt: %s\n", debugReq.Messages[0].Content)
}

// HandleRequest returns a handler function that processes incoming requests
func HandleRequest(backend, modelName, promptsDir, apiKey, apiBase string, debug bool, enableThinking bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for all responses
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Only accept GET and POST requests
		if r.Method != "GET" && r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the URL path to get the prompt file name
		promptFile := strings.TrimPrefix(r.URL.Path, "/")
		if promptFile == "" {
			promptFile = "home"
		}

		// Add .txt extension if not present
		if !strings.HasSuffix(promptFile, ".txt") {
			promptFile += ".txt"
		}

		// Construct the full path to the prompt file
		promptPath := filepath.Join(promptsDir, promptFile)

		// Check if the file exists
		if _, err := os.Stat(promptPath); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("Prompt file not found: %s", promptFile), http.StatusNotFound)
			return
		}

		// Read the prompt file
		promptData, err := os.ReadFile(promptPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading prompt file: %v", err), http.StatusInternalServerError)
			return
		}

		// Extract system prompt and user prompt from the file
		// The first line is the system prompt, the rest is the user prompt
		promptContent := string(promptData)
		lines := strings.SplitN(promptContent, "\n", 2)

		var systemPrompt, userPrompt string
		if len(lines) > 0 {
			systemPrompt = lines[0]
		}
		if len(lines) > 1 {
			userPrompt = lines[1]
		}

		// Get user input from POST data if available
		if r.Method == "POST" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			userInput := string(body)
			if userInput != "" {
				userPrompt += "\n\nUser Input: " + userInput
			}
		}

		// Print debug information if enabled
		if debug {
			PrintRequestDebugInfo(backend, modelName, systemPrompt, userPrompt, enableThinking)
		}

		// Set content type for streaming response
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Get flusher for streaming
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Create model handler based on backend
		handler := models.NewModelHandler(backend, modelName, apiKey, apiBase, enableThinking)

		// Stream the response
		err = handler.StreamResponse(w, flusher, systemPrompt, userPrompt)
		if err != nil {
			log.Printf("Error streaming response: %v", err)
			// Don't send an error response here as we may have already started streaming
		}
	}
}
