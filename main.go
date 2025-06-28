package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kekePower/museweb/pkg/config"
	"github.com/kekePower/museweb/pkg/errors"
	"github.com/kekePower/museweb/pkg/middleware"
	"github.com/kekePower/museweb/pkg/server"
	"github.com/kekePower/museweb/pkg/utils"
)

const version = "1.2.0-dev"

func main() {
	// --- Load Configuration ---
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Printf("⚠️  Could not load config.yaml: %v. Using defaults and flags only.", err)
	}

	// Set reasoning model patterns from configuration
	if len(cfg.Model.ReasoningModels) > 0 {
		utils.SetReasoningModelPatterns(cfg.Model.ReasoningModels)
		log.Printf("🧠 Loaded %d reasoning model patterns from config", len(cfg.Model.ReasoningModels))
	}

	// --- Define Command-Line Flags ---
	showVersion := flag.Bool("version", false, "Display the version and exit")
	host := flag.String("host", cfg.Server.Address, "Interface to bind to (e.g., 127.0.0.1 or 0.0.0.0)")
	port := flag.String("port", cfg.Server.Port, "Port to run the web server on")
	promptsDir := flag.String("prompts", cfg.Server.PromptsDir, "Directory containing prompt files")
	backend := flag.String("backend", cfg.Model.Backend, "AI backend to use (ollama or openai)")
	model := flag.String("model", cfg.Model.Name, "Model name to use")
	// Default API key based on backend
	var defaultAPIKey string
	if strings.ToLower(cfg.Model.Backend) == "openai" {
		defaultAPIKey = cfg.OpenAI.APIKey
	} else {
		defaultAPIKey = cfg.Ollama.APIKey
	}
	apiKey := flag.String("api-key", defaultAPIKey, "API key for the selected backend (ignored if not required)")

	// Choose sensible default for api-base depending on backend in config
	var defaultAPIBase string
	if strings.ToLower(cfg.Model.Backend) == "openai" {
		defaultAPIBase = cfg.OpenAI.APIBase
	} else {
		defaultAPIBase = cfg.Ollama.APIBase
	}
	apiBase := flag.String("api-base", defaultAPIBase, "Base URL for the selected backend")
	debug := flag.Bool("debug", cfg.Server.Debug, "Enable debug mode")
	flag.Parse()

	if *showVersion {
		fmt.Printf("MuseWeb v%s\n", version)
		os.Exit(0)
	}

	// --- Final Configuration ---
	// If the api-key flag is still empty, try backend-specific environment variable as a last resort.
	if *apiKey == "" {
		if strings.ToLower(*backend) == "openai" {
			*apiKey = os.Getenv("OPENAI_API_KEY")
		} else {
			*apiKey = os.Getenv("OLLAMA_API_KEY")
		}
	}

	// --- Validate OpenAI Config ---
	if *backend == "openai" && *apiKey == "" {
		log.Fatalf("❌ For the 'openai' backend, the API key must be provided via the -api-key flag, the config.yaml file, or the OPENAI_API_KEY environment variable.")
	}

	// --- Setup HTTP Server ---
	serverHandler := server.HandleRequest(*backend, *model, *promptsDir, *apiKey, *apiBase, *debug)

	// Main route handler with recovery middleware
	mainHandler := middleware.WrapHandler(func(w http.ResponseWriter, r *http.Request) {
		// Serve static files if the path contains a dot (file extension)
		if strings.Contains(r.URL.Path, ".") {
			// Determine static file paths
			staticReqPath := strings.TrimPrefix(r.URL.Path, "/") // e.g. "logo.png" or "static/logo.png"
			promptScopedPath := filepath.Join(*promptsDir, "public", staticReqPath)
			globalPath := filepath.Join("public", staticReqPath)

			// Try prompt-scoped public directory first
			if _, err := os.Stat(promptScopedPath); err == nil {
				http.ServeFile(w, r, promptScopedPath)
				return
			}
			// Fall back to global public directory
			if _, err := os.Stat(globalPath); err == nil {
				http.ServeFile(w, r, globalPath)
				return
			}
			// Not found in either location
			errors.RenderErrorPage(w, r, http.StatusNotFound, fmt.Sprintf("Static file '%s' not found in prompt-scoped or global public directories", r.URL.Path))
			return
		}
		// Otherwise, handle as a prompt request
		serverHandler.ServeHTTP(w, r)
	})

	http.HandleFunc("/", mainHandler)

	displayHost := *host
	if *host == "0.0.0.0" {
		displayHost = "localhost"
	}

	listenAddr := *host
	if listenAddr == "0.0.0.0" {
		listenAddr = ""
	}

	// Add a test route for error handling (can be removed in production)
	if *debug {
		http.HandleFunc("/error-test", middleware.WrapHandler(func(w http.ResponseWriter, r *http.Request) {
			// Test different error types based on query parameter
			errorType := r.URL.Query().Get("type")
			switch errorType {
			case "panic":
				panic("Test panic for error handling")
			case "404":
				errors.NotFound(w, r)
			case "500":
				errors.InternalServerError(w, r, "Test internal server error")
			case "405":
				errors.MethodNotAllowed(w, r)
			default:
				errors.BadRequest(w, r, "Invalid error type. Use: panic, 404, 500, or 405")
			}
		}))
		log.Printf("📝 Debug mode: Error testing available at /error-test?type=[panic|404|500|405]")
	}

	// Create a custom HTTP server with longer timeouts for AI responses
	server := &http.Server{
		Addr:         listenAddr + ":" + *port,
		ReadTimeout:  60 * time.Second,  // Time to read request
		WriteTimeout: 300 * time.Second, // Time to write response (5 minutes for large AI responses)
		IdleTimeout:  120 * time.Second, // Time to keep connections alive
	}

	log.Printf("✨ MuseWeb v%s is live at http://%s:%s", version, displayHost, *port)
	log.Printf("   (Using backend '%s', model '%s', and prompts from '%s')", *backend, *model, *promptsDir)
	if utils.IsThinkingEnabledModel(*model) {
		log.Printf("   🧠 Thinking tag enabled for %s model", *model)
	}

	err = server.ListenAndServe()
	if err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}
