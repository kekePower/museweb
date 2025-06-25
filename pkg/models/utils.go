package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kekePower/museweb/pkg/utils"
)

// tryDirectRequest attempts to make a direct HTTP request to the API
// This is used as a fallback when the OpenAI client fails to create a stream
func tryDirectRequest(apiBase, apiKey, modelName, systemPrompt, userPrompt string, debug bool) (string, error) {
	log.Printf("[DEBUG] Attempting direct request to %s with model %s", apiBase, modelName)
	
	// Ensure BaseURL ends with /v1 as required by OpenAI-compatible endpoints
	if !strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimRight(apiBase, "/") + "/v1"
	}
	
	// Construct the request URL
	url := apiBase + "/chat/completions"
	
	// Construct the request body
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": false, // Don't stream for diagnostic request
	}
	
	// Marshal the request body to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request body: %w", err)
	}
	
	// Create the HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	
	// Create a custom HTTP client with optional debug transport
	var client *http.Client
	if debug {
		client = &http.Client{
			Transport: &utils.DebugTransport{
				Transport: http.DefaultTransport,
			},
			Timeout: 2 * time.Minute, // Increased from 30 seconds to handle large responses
		}
		log.Printf("[DEBUG] HTTP debugging enabled for direct request")
	} else {
		client = &http.Client{
			Transport: http.DefaultTransport,
			Timeout: 2 * time.Minute, // Increased from 30 seconds to handle large responses
		}
	}
	
	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return "", fmt.Errorf("I/O timeout: %w", err)
		}
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return "", fmt.Errorf("I/O timeout: %w", err)
		}
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	
	// Log response status and headers
	log.Printf("[DEBUG] Direct request status: %s", resp.Status)
	log.Printf("[DEBUG] Direct request headers: %v", resp.Header)
	
	// Check for non-200 status code
	if resp.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("API returned non-200 status: %s - %s", resp.Status, string(body))
	}
	
	// Process the response to handle non-standard content format
	processedBody := utils.UnwrapContentStringField(string(body))
	
	return processedBody, nil
}
