package models

import (
	"context"
	"io"
	"log"
	"net/http"
)

// OpenAIHandler implements the ModelHandler interface for OpenAI-compatible APIs
type OpenAIHandler struct {
	ModelName string
	APIKey    string
	APIBase   string
	Debug     bool
}

// StreamResponse streams the response from the OpenAI model
func (h *OpenAIHandler) StreamResponse(w io.Writer, flusher http.Flusher, systemPrompt, userPrompt string) error {
	ctx := context.Background()

	if h.Debug {
		log.Printf("[DEBUG] Creating OpenAI stream with model: %s, API base: %s", h.ModelName, h.APIBase)
	}

	// Always use handleWithCustomRequest for reasoning models
	return h.handleWithCustomRequest(ctx, w, flusher, systemPrompt, userPrompt)
}
