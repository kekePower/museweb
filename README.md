# MuseWeb

MuseWeb is an **experimental, prompt-driven web server** that streams HTML straight from plain-text prompts using a large-language model (LLM). Originally built "just for fun," it currently serves as a proof-of-concept for what prompt-driven websites could become once local LLMs are fast and inexpensive. Even in this early state, it showcases the endless possibilities of minimal, fully self-hosted publishing.

**Version 1.1.0** introduces a complete modular architecture for improved maintainability and extensibility.

---

## ✨ Features

* **Prompt → Page** – Point MuseWeb to a folder of `.txt` prompts; each prompt becomes a routable page.
* **Live Reloading for Prompts** – Edit your prompt files and see changes instantly without restarting the server.
* **Streaming Responses** – HTML is streamed token-by-token for instant first paint.
* **Backend Agnostic** – Works with either:
  * **[Ollama](https://ollama.ai/)** (default, runs everything locally), or
  * Any **OpenAI-compatible** API (e.g. OpenAI, Together.ai, Groq, etc.).
* **Single Binary** – Go-powered, ~7 MB static binary, no external runtime.
* **Zero JS by Default** – Only the streamed HTML from the model is served; you can add your own assets in `public/`.
* **Modular Architecture** – Clean separation of concerns with dedicated packages for configuration, server, models, and utilities.
* **Configurable via `config.yaml`** – Port, model, backend, prompt directory, and OpenAI credentials.
* **Environment Variable Support** – Falls back to `OPENAI_API_KEY` if not specified in config or flags.
* **Reasoning Model Support** – Automatic detection and handling of reasoning models (DeepSeek, R1, Qwen, etc.) with thinking output disabled for clean web pages.
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
openai:
  api_key: ""           # Required when backend = "openai"
  api_base: "https://api.openai.com/v1" # Change for other providers
```

Configuration can be overridden with CLI flags:

```bash
# Example with command-line flags
./museweb -port 9000 -model mistral -backend ollama -debug

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

## 🏗️ Architecture

As of version 1.1.0, MuseWeb has been fully modularized with a clean separation of concerns:

```
/
├── main.go           # Application entry point and orchestration
├── config.yaml       # Configuration file
├── public/           # Static files served directly
├── prompts/          # Prompt text files
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
