package utils

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

// DebugTransport is an http.RoundTripper that logs requests and responses
type DebugTransport struct {
	Transport http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface
func (d *DebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log the request
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.Printf("[DEBUG] Failed to dump request: %v", err)
	} else {
		// Redact Authorization header for security
		log.Printf("[DEBUG] HTTP Request: %s", redactAuthHeader(reqDump))
	}

	// Record the time before the request
	startTime := time.Now()

	// Perform the request
	resp, err := d.Transport.RoundTrip(req)
	if err != nil {
		log.Printf("[DEBUG] HTTP Error: %v", err)
		return nil, err
	}

	// Calculate request duration
	duration := time.Since(startTime)
	log.Printf("[DEBUG] Request took %v", duration)

	// Log the response
	respDump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Printf("[DEBUG] Failed to dump response: %v", err)
	} else {
		log.Printf("[DEBUG] HTTP Response: %s", respDump)
	}

	// Create a copy of the response with a new body that we can read and then restore
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[DEBUG] Failed to read response body: %v", err)
		return resp, nil
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the response body separately for better readability
	log.Printf("[DEBUG] Response Body: %s", bodyBytes)

	return resp, nil
}

// redactAuthHeader replaces the Authorization header value with "REDACTED"
func redactAuthHeader(dump []byte) []byte {
	lines := bytes.Split(dump, []byte("\r\n"))
	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("Authorization:")) {
			lines[i] = []byte("Authorization: Bearer REDACTED")
		}
	}
	return bytes.Join(lines, []byte("\r\n"))
}

// NewDebugClient creates an http.Client with request/response logging
func NewDebugClient() *http.Client {
	return &http.Client{
		Transport: &DebugTransport{
			Transport: http.DefaultTransport,
		},
	}
}

// MakeDebugRequest makes an HTTP request with full debugging
func MakeDebugRequest(method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte, error) {
	// Create the request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Create debug client
	client := NewDebugClient()

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("error reading response body: %w", err)
	}

	return resp, respBody, nil
}
