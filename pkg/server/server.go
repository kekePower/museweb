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
func PrintRequestDebugInfo(backend, modelName, systemPrompt, userPrompt string, disableThinking bool) {
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
		Thinking: !disableThinking,
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
func HandleRequest(backend, modelName, promptsDir, apiKey, apiBase string, debug bool) http.HandlerFunc {
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
		originalPath := r.URL.Path
		promptFile := strings.TrimPrefix(originalPath, "/")
		// Remove trailing slash if present (AI sometimes generates URLs like /path/?lang=xx)
		promptFile = strings.TrimSuffix(promptFile, "/")
		if promptFile == "" {
			promptFile = "home"
		}
		
		// Debug logging for URL path cleaning
		if debug && strings.HasSuffix(originalPath, "/") && originalPath != "/" {
			log.Printf("🔧 Cleaned URL path: '%s' -> '%s'", originalPath, promptFile)
		}

		// Extract language parameter from URL query string
		langParam := r.URL.Query().Get("lang")
		if debug && langParam != "" {
			log.Printf("🌐 Language parameter detected: %s", langParam)
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

		// Load the system prompt from system_prompt.txt
		systemPromptPath := filepath.Join(promptsDir, "system_prompt.txt")
		var systemPrompt string

		// Check if system_prompt.txt exists
		if _, err := os.Stat(systemPromptPath); !os.IsNotExist(err) {
			// Read the system prompt file
			systemPromptData, err := os.ReadFile(systemPromptPath)
			if err != nil {
				log.Printf("Warning: Error reading system_prompt.txt: %v", err)
			} else {
				systemPrompt = string(systemPromptData)
			}
		} else {
			log.Printf("Warning: system_prompt.txt not found in %s", promptsDir)
		}

		// Check for layout files
		layoutMinPath := filepath.Join(promptsDir, "layout.min.txt")
		layoutPath := filepath.Join(promptsDir, "layout.txt")
		var layoutContent string

		// First try layout.min.txt, then fall back to layout.txt
		if _, err := os.Stat(layoutMinPath); !os.IsNotExist(err) {
			layoutData, err := os.ReadFile(layoutMinPath)
			if err == nil {
				layoutContent = string(layoutData)
			}
		} else if _, err := os.Stat(layoutPath); !os.IsNotExist(err) {
			layoutData, err := os.ReadFile(layoutPath)
			if err == nil {
				layoutContent = string(layoutData)
			}
		}

		// If we have a layout, append it to the system prompt
		if layoutContent != "" {
			if systemPrompt != "" {
				systemPrompt += "\n\n" + layoutContent
			} else {
				systemPrompt = layoutContent
			}
		}

		// The prompt file content becomes the user prompt
		userPrompt := string(promptData)

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

		// Add translation instruction if language parameter is provided
		if langParam != "" {
			// Validate and clean the language parameter (basic sanitization)
			langParam = strings.TrimSpace(langParam)
			if len(langParam) > 0 && len(langParam) <= 10 { // Reasonable length limit
				translationInstruction := fmt.Sprintf("\n\nTranslate all the content to %s.\n**VERY IMPORTANT:** DO NOT TRANSLATE ANY OF THE URLS IN THE NAVBAR. Keep the links as they are.\n**VERY IMPORTANT:** Add ?lang=%s to all generated URLs to preserve the language context.", langParam, langParam)
				userPrompt += translationInstruction
				if debug {
					log.Printf("🌐 Added translation instruction: %s", translationInstruction)
				}
			} else if debug {
				log.Printf("⚠️  Invalid language parameter ignored: %s", langParam)
			}
		}

		// Print debug information if enabled
		if debug {
			PrintRequestDebugInfo(backend, modelName, systemPrompt, userPrompt, false)
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
		handler := models.NewModelHandler(backend, modelName, apiKey, apiBase, debug)

		// Stream the response
		err = handler.StreamResponse(w, flusher, systemPrompt, userPrompt)
		if err != nil {
			log.Printf("Error streaming response: %v", err)
			// Don't send an error response here as we may have already started streaming
		}
	}
}
