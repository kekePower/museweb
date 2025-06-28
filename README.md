# MuseWeb

MuseWeb is an **experimental, prompt-driven web server** that streams HTML straight from plain-text prompts using a large-language model (LLM). **Works with any OpenAI-compatible API** - from local Ollama models to cloud providers like OpenAI, Anthropic, Google, Together.ai, Groq, and hundreds more. Originally built "just for fun," it currently serves as a proof-of-concept for what prompt-driven websites could become once local LLMs are fast and inexpensive. Even in this early state, it showcases the endless possibilities of minimal, fully self-hosted publishing.

**Version 1.1.4** introduces enhanced model support, robust output sanitization, and critical streaming fixes for clean HTML generation.

---

## ✨ Features

* **Prompt → Page** – Point MuseWeb to a folder of `.txt` prompts; each prompt becomes a routable page.
* **Live Reloading for Prompts** – Edit your prompt files and see changes instantly without restarting the server.
* **Streaming Responses** – HTML is streamed token-by-token for instant first paint with real-time sanitization.
* **Universal API Compatibility** – Works with **any OpenAI-compatible API endpoint**:
  * **[Ollama](https://ollama.ai/)** (default, runs everything locally)
  * **OpenAI** (GPT-4, GPT-3.5, etc.)
  * **Anthropic Claude** (via OpenAI-compatible proxies)
  * **Google Gemini** (via OpenAI-compatible endpoints)
  * **Together.ai** (hundreds of open-source models)
  * **Groq** (ultra-fast inference)
  * **Inception Labs Mercury** (advanced reasoning models)
  * **Perplexity** (Sonar models with web search)
  * **Novita.ai** (global model marketplace)
  * **OpenRouter** (unified API for 200+ models)
  * **Local providers** (LM Studio, vLLM, Text Generation WebUI, etc.)
  * **Any other OpenAI-compatible endpoint** – Just change the `api_base` URL!
* **Single Binary** – Go-powered, ~7 MB static binary, no external runtime.
* **Zero JS by Default** – Only the streamed HTML from the model is served; you can add your own assets in `public/`.
* **Modular Architecture** – Clean separation of concerns with dedicated packages for configuration, server, models, and utilities.
* **Prompt-Scoped Static Assets** – Each prompt set can have its own `public/` directory for static files (CSS, images, JS, etc.), with automatic resolution and fallback to the global `public/` directory.
* **Robust Output Sanitization** – Advanced code fence removal and markdown artifact cleaning for pristine HTML output.
* **Enhanced Model Support** – Comprehensive support for reasoning models including DeepSeek, R1, Qwen, Mercury, and more.
* **Configurable via `config.yaml`** – Port, model, backend, prompt directory, and API credentials.
* **Environment Variable Support** – Falls back to `OPENAI_API_KEY` if not specified in config or flags.
* **Reasoning Model Support** – Automatic detection and handling of reasoning models with thinking output disabled for clean web pages.
* **Detailed Logging** – Comprehensive logging of prompt file loading and request handling for easy debugging.

---

## 🚀 Quick Start

```bash
# 1. Clone and build
$ git clone https://github.com/kekePower/museweb.git
$ cd museweb
$ GO111MODULE=on go build .

# 2. (Optional) pull an LLM with Ollama
$ ollama pull llama3

# 3. Run with defaults (localhost:8080)
$ ./museweb
```

Open <http://localhost:8080> in your browser. Navigation links are generated from the prompt filenames.

---

## 🔧 Configuration

Copy `config.example.yaml` to `config.yaml` and tweak as needed:

```yaml
server:
  address: "127.0.0.1"  # Interface to bind to (e.g., 127.0.0.1 or 0.0.0.0)
  port: "8080"          # Port for HTTP server
  prompts_dir: "./prompts"  # Folder containing *.txt prompt files
  debug: false          # Enable debug logging
model:
  backend: "ollama"     # "ollama" or "openai"
  name: "llama3"        # Model name to use
  reasoning_models:     # Patterns for reasoning models (thinking disabled automatically)
    - "deepseek"
    - "r1-1776"
    - "qwen"
    - "mercury"
openai:
  api_key: ""           # Required when backend = "openai"
  api_base: "https://api.openai.com/v1" # Universal: works with ANY OpenAI-compatible API!

### 🌐 Universal API Compatibility Examples:

```yaml
# OpenAI (official)
api_base: "https://api.openai.com/v1"

# Together.ai (200+ open-source models)
api_base: "https://api.together.xyz/v1"

# Groq (ultra-fast inference)
api_base: "https://api.groq.com/openai/v1"

# OpenRouter (unified API for 200+ models)
api_base: "https://openrouter.ai/api/v1"

# Perplexity (Sonar models with web search)
api_base: "https://api.perplexity.ai"

# Local LM Studio
api_base: "http://localhost:1234/v1"

# Local vLLM server
api_base: "http://localhost:8000/v1"

# Any other OpenAI-compatible endpoint
api_base: "https://your-provider.com/v1"
```

Configuration can be overridden with CLI flags:

```bash
# Example with command-line flags
./museweb -port 9000 -model mistral -backend ollama -debug

# Connect to any OpenAI-compatible provider
./museweb -backend openai -api-base "https://api.together.xyz/v1" -model "meta-llama/Llama-3.2-11B-Vision-Instruct-Turbo"

# Use local LM Studio
./museweb -backend openai -api-base "http://localhost:1234/v1" -model "llama-3.2-3b-instruct"

# View all available options
./museweb -h
```

For OpenAI API keys, MuseWeb will check these sources in order:
1. Command-line flag (`-api-key`)
2. Configuration file (`config.yaml`)
3. Environment variable (`OPENAI_API_KEY`)

---

## 📝 Writing Prompts

* Place text files in the prompts directory – `home.txt`, `about.txt`, etc.
* The filename (without extension) becomes the route: `about.txt → /about`.
* **`system_prompt.txt` is the only file that *must* exist.** Define your site's core rules, output protocols, and structural requirements here.
* **`layout.txt` is a special file** that gets appended to the system prompt for all pages. Use it to define global layout, styling, and interactive elements that should be consistent across all pages.
* **`layout.min.txt` is an optional alternative** to `layout.txt` that produces minified HTML output, saving tokens and reducing response size. The server will use this file instead of `layout.txt` if it exists.
* All prompt files are loaded from disk on every request, so you can edit them and see changes without restarting the server.
* The prompt files included in this repo are **examples only**—update or replace them to suit your own site.
* HTML, Markdown, or plain prose inside the prompt will be passed verbatim to the model – **sanitize accordingly before publishing**.
* For best results, keep design instructions in `layout.txt` and focus content instructions in individual page prompts.

---

## 🧹 Output Sanitization

MuseWeb includes robust output sanitization to ensure clean HTML generation from AI models:

### Automatic Code Fence Removal
* **Real-time cleaning** – Code fences (````html`, `````, etc.) are removed during streaming for immediate clean output
* **Universal application** – Works with all models including those that ignore prompt instructions about code formatting
* **Comprehensive patterns** – Handles various code fence formats: ````html`, ````HTML`, `````, and standalone `html` text
* **Safe processing** – Preserves valid HTML content while removing only markdown artifacts

### Model-Specific Handling
* **Mercury models** (Inception Labs) – Specialized handling for models that persistently wrap HTML in code fences
* **Reasoning models** – Automatic detection and sanitization of thinking tags and reasoning output
* **Streaming architecture** – Sanitization occurs before content reaches the client, not after

### Advanced Features
* **Multi-layer cleaning** – Sequential processing with regex patterns inspired by proven markdown strippers
* **Whitespace preservation** – Maintains important spacing between HTML elements during streaming
* **Edge case handling** – Removes standalone artifacts like orphaned `html` text without breaking valid content

This ensures that regardless of which AI model you use, MuseWeb delivers clean, properly formatted HTML to your visitors.

---

## 📚 Examples

The `examples/` directory contains 4 complete website templates showcasing different styles and approaches:

### Available Examples

* **`minimalist/`** – Clean, minimal design focused on typography and whitespace
* **`corporate/`** – Professional business website with multiple pages and corporate styling  
* **`fantasy/`** – Creative fantasy-themed site with rich imagery and atmospheric design
* **`98retro/`** – Nostalgic late-90s web aesthetic with retro styling and design elements

### Using Examples

Each example is a complete website template with:
- `system_prompt.txt` – Core instructions and site personality
- `layout.txt` – Global layout and styling definitions
- Page prompts (e.g., `home.txt`, `about.txt`) – Individual page content
- `public/` directory – CSS files and assets specific to that theme

**Prompt-Scoped Static Assets**

As of v1.2.0, each prompt set can have its own `public/` directory for static files (CSS, images, JS, etc.). When a static file is requested:

1. MuseWeb first checks for the file in the active prompt set's `public/` directory (e.g. `prompts/corporate/public/logo.png`).
2. If not found, it falls back to the global `public/` directory (e.g. `public/logo.png`).
3. If still not found, a custom 404 error page is shown.

**To use an example:**

1. Copy the example's prompt files to your main `prompts/` directory:
   ```bash
   cp -r examples/minimalist prompts/minimalist
   ```

2. Run MuseWeb with that prompt set:
   ```bash
   ./museweb -prompts prompts/minimalist
   ```

3. Place any custom assets for that prompt set in `prompts/minimalist/public/`. Place global assets in `public/`.

4. When requesting `/logo.png`, MuseWeb will serve `prompts/minimalist/public/logo.png` if it exists, otherwise fall back to `public/logo.png`.

**No need to copy assets from example public/ to global public/ anymore!**

---

## 🏗️ Architecture

As of version 1.1.4, MuseWeb has been fully modularized with a clean separation of concerns:

```
/
├── main.go           # Application entry point and orchestration
├── config.yaml       # Configuration file
├── public/           # Global static files (fallback for all prompts)
├── prompts/          # Prompt text files
│   └── [prompt-set]/public/  # Prompt-scoped static files (served for that prompt set only)
└── pkg/              # Go packages
    ├── config/       # Configuration loading and validation
    ├── models/       # AI model backends (Ollama and OpenAI)
    ├── server/       # HTTP server and request handling
    └── utils/        # Utility functions for output processing
```

### Key Components

* **Configuration**: The `config` package handles loading settings from YAML with sensible defaults.
* **Model Abstraction**: The `models` package provides a common interface for different AI backends.
* **HTTP Server**: The `server` package manages HTTP requests, static file serving, and prompt processing.
* **Utilities**: The `utils` package contains functions for sanitizing and processing model outputs.

## 🤝 Contributing

1. Fork the repo and create a feature branch.
2. Run `go vet ./... && go test ./...` before opening a PR.
3. Follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.

Bug reports and feature ideas are very welcome! 🙏

---

## 📜 License

MuseWeb is distributed under the terms of the Apache License, Version 2.0. See the `LICENSE` file for full details.
