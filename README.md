# MuseWeb

MuseWeb is an **experimental, prompt-driven web server** that streams HTML straight from plain-text prompts using a large-language model (LLM). Originally built “just for fun,” it currently serves as a proof-of-concept for what prompt-driven websites could become once local LLMs are fast and inexpensive. Even in this early state, it showcases the endless possibilities of minimal, fully self-hosted publishing.

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
* **Configurable via `config.yaml`** – Port, model, backend, prompt directory, and OpenAI credentials.
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
  port: "8080"         # Port for HTTP server
  prompts_dir: "./prompts"  # Folder containing *.txt prompt files
model:
  backend: "ollama"    # "ollama" or "openai"
  name: "llama3"       # Model name to use
openai:
  api_key: ""          # Required when backend = "openai"
  api_base: "https://api.openai.com/v1" # Change for other providers
```

Configuration can be overridden with CLI flags, e.g. `./museweb -port 9000 -model mistral`

---

## 📝 Writing Prompts

* Place text files in the prompts directory – `home.txt`, `about.txt`, etc.
* The filename (without extension) becomes the route: `about.txt → /about`.
* **`system_prompt.txt` is the only file that *must* exist.** Define your site's core rules and even entire pages inside this file if you want.
* **`layout.txt` is a special file** that gets appended to the system prompt for all pages. Use it to define global layout, styling, and interactive elements that should be consistent across all pages.
* All prompt files are loaded from disk on every request, so you can edit them and see changes without restarting the server.
* The prompt files included in this repo are **examples only**—update or replace them to suit your own site.
* HTML, Markdown, or plain prose inside the prompt will be passed verbatim to the model – **sanitize accordingly before publishing**.
* For best results, keep design instructions in `layout.txt` and focus content instructions in individual page prompts.

---

## 🤝 Contributing

1. Fork the repo and create a feature branch.
2. Run `go vet ./... && go test ./...` before opening a PR.
3. Follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.

Bug reports and feature ideas are very welcome! 🙏

---

## 📜 License

MuseWeb is distributed under the terms of the Apache License, Version 2.0. See the `LICENSE` file for full details.
