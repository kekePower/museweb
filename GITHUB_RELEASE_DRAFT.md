# MuseWeb v1.1.4 - Universal API Compatibility & Robust Sanitization

## 🌟 Major Features

### 🌐 **Universal OpenAI API Compatibility**
MuseWeb now works with **ANY OpenAI-compatible API endpoint**:
- **Cloud**: OpenAI, Anthropic, Google Gemini, Together.ai, Groq, Perplexity, OpenRouter
- **Local**: Ollama, LM Studio, vLLM, Text Generation WebUI
- **Just change the `api_base` URL** - no code changes needed!

### 🧹 **Robust Output Sanitization**
Advanced cleaning system for problematic AI model outputs:
- **Real-time code fence removal** during streaming
- **Mercury model support** - handles models that ignore prompt instructions
- **Universal application** across all providers
- **Preserves valid HTML** while removing markdown artifacts

### 🔧 **Critical Streaming Fixes**
Fixed fundamental bugs causing duplicate content:
- **Stream-time sanitization** - content cleaned BEFORE sending to client
- **No more duplicate responses** - eliminated double-write bugs
- **5-minute HTTP timeout** for large responses (300KB+)

## 🚀 **Key Improvements**

- ✅ **Enhanced Model Support**: Mercury, DeepSeek R1, priority-based reasoning detection
- ✅ **Performance Optimizations**: Compressed prompts, efficient streaming
- ✅ **Developer Experience**: Comprehensive docs, practical CLI examples
- ✅ **Configuration Flexibility**: YAML + CLI flags + environment variables

## 🐛 **Critical Bug Fixes**

- **Streaming Duplicate Content**: Fixed double-write causing duplicate responses
- **Mercury Code Fences**: Universal removal of ```html markdown artifacts
- **HTTP Timeouts**: Extended timeout for large AI responses
- **Model Detection**: Priority-based system prevents pattern conflicts

## 📋 **Supported Providers**

```bash
# OpenAI (official)
./museweb -backend openai -api-base "https://api.openai.com/v1"

# Together.ai (200+ models)
./museweb -backend openai -api-base "https://api.together.xyz/v1"

# Groq (ultra-fast)
./museweb -backend openai -api-base "https://api.groq.com/openai/v1"

# Local LM Studio
./museweb -backend openai -api-base "http://localhost:1234/v1"

# Any other OpenAI-compatible endpoint
./museweb -backend openai -api-base "https://your-provider.com/v1"
```

## 🔄 **Migration**

- ✅ **No breaking changes** - existing configurations work as-is
- ✅ **Enhanced functionality** - new features are additive
- ✅ **Improved stability** - bug fixes improve existing behavior

## 🧪 **Tested Scenarios**

- ✅ Mercury models with clean HTML output
- ✅ 300KB+ responses without timeouts
- ✅ Multiple providers (OpenAI, Together.ai, Groq, Ollama)
- ✅ Real-time streaming sanitization
- ✅ Reasoning model detection (DeepSeek R1, Mercury, Qwen)

## 🎯 **What This Enables**

- **Provider Freedom**: Use any AI provider without vendor lock-in
- **Clean Output**: Professional HTML regardless of model quirks
- **Reliable Streaming**: No more duplicate or broken responses
- **Universal Integration**: One codebase, any OpenAI-compatible API

---

**Full Release Notes**: [RELEASE_NOTES_1.1.4.md](RELEASE_NOTES_1.1.4.md)  
**Full Changelog**: https://github.com/kekePower/museweb/compare/v1.1.3...v1.1.4
