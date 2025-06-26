// Package models provides interfaces and implementations for AI model handlers
package models

// This file serves as the main entry point for the models package.
// It re-exports the public API that other parts of the application need.
//
// The implementation details are split into separate files:
// - interface.go: Contains the ModelHandler interface definition
// - ollama.go: Contains the Ollama implementation
// - openai.go: Contains the OpenAI implementation
// - openai_custom.go: Contains custom request handling for OpenAI
// - transport.go: Contains HTTP transport utilities
// - utils.go: Contains common utility functions

// NewModelHandler creates a new model handler based on the backend type
// This is the main factory function that external code should use to create model handlers
func NewModelHandler(backend, modelName, apiKey, apiBase string, debug bool) ModelHandler {
	// Implementation is in interface.go
	return newModelHandler(backend, modelName, apiKey, apiBase, debug)
}
