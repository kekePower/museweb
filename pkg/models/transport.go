package models

import (
	"net/http"
)

// customHeaderTransport is a custom http.RoundTripper that adds headers to requests
type customHeaderTransport struct {
	base     http.RoundTripper
	thinking bool
}

// authTransport adds Bearer token for Ollama requests when API key provided
type authTransport struct {
	base   http.RoundTripper
	apiKey string
}

// RoundTrip implements the http.RoundTripper interface for authTransport
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}
	return t.base.RoundTrip(req)
}

// RoundTrip implements the http.RoundTripper interface for customHeaderTransport
func (t *customHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add custom headers to the request
	if t.thinking {
		req.Header.Set("X-Thinking-Enabled", "true")
	}
	// Use the base transport to perform the actual request
	return t.base.RoundTrip(req)
}
