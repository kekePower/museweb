package models

import (
	"io"
	"net/http"
)

// ModelHandler is an interface for different AI model backends
type ModelHandler interface {
	StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error
}

// newModelHandler creates a new model handler based on the backend type
// This is an internal implementation function called by the public NewModelHandler in models.go
func newModelHandler(backend, modelName, apiKey, apiBase string, debug bool) ModelHandler {
	switch backend {
	case "openai":
		return &OpenAIHandler{
			ModelName: modelName,
			APIKey:    apiKey,
			APIBase:   apiBase,
			Debug:     debug,
		}
	default:
		return &OllamaHandler{
			ModelName:       modelName,
			APIKey:          apiKey,
			APIBase:         apiBase,
			DisableThinking: false, // Keep for Ollama handler
			Debug:           debug,
		}
	}
}
