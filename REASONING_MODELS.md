# Reasoning Models in MuseWeb

MuseWeb automatically detects and handles models with reasoning/thinking capabilities. For web page generation, thinking output is automatically disabled to ensure clean HTML responses.

## How It Works

When MuseWeb detects a reasoning model (based on configurable patterns), it automatically:
1. **Sends `thinking: false`** in the API request to disable reasoning output
2. **Ensures clean responses** without thinking tags cluttering the web page
3. **Maintains full functionality** while providing a better user experience

## Supported Models

The following model patterns are automatically detected as reasoning models:

### DeepSeek Models
- `deepseek-r1-distill` - DeepSeek R1 distilled models (most specific)
- `deepseek` - All DeepSeek models (general)

### OpenAI R1 Models  
- `r1-1776` - OpenAI R1-1776 models

### Qwen Models
- `qwen3` - Qwen3 models (specific)
- `qwen` - All Qwen models (general)

### Perplexity Models
- `sonar-reasoning-pro` - Perplexity Sonar reasoning pro models
- `sonar-reasoning` - Perplexity Sonar reasoning models

### Gemini Models
- `gemini-2.5-flash-lite-preview-06-17` - Specific Gemini model
- `gemini-2.5-flash` - Gemini 2.5 Flash models

## Configuration

You can customize which models are treated as reasoning models by editing the `reasoning_models` section in your `config.yaml`:

```yaml
model:
  reasoning_models:
    - "deepseek-r1-distill"  # Most specific patterns first
    - "sonar-reasoning-pro"
    - "sonar-reasoning"
    - "r1-1776"
    - "qwen3"
    - "deepseek"             # General patterns last
    - "qwen"
```

### Pattern Matching Rules

1. **Case-insensitive**: Patterns match regardless of case
2. **Substring matching**: Pattern must appear anywhere in the model name
3. **Priority-based**: First match wins, so list specific patterns before general ones
4. **Automatic thinking disabled**: All matched models get `thinking: false` automatically

## Why Thinking is Disabled

For web page generation, reasoning output creates several problems:
- **Cluttered HTML**: Thinking tags appear in the generated web page
- **Poor UX**: Users see the model's internal reasoning process
- **Inconsistent output**: Some responses have thinking, others don't
- **Parsing issues**: Thinking tags can interfere with HTML structure

By automatically disabling thinking for reasoning models, MuseWeb ensures:
- **Clean web pages** without reasoning clutter
- **Consistent output** across all requests  
- **Better user experience** with focused content
- **Proper HTML structure** without interference

## Adding New Models

To add support for new reasoning models, simply add their name patterns to the `reasoning_models` list in your config:

```yaml
model:
  reasoning_models:
    - "new-reasoning-model"  # Add your pattern here
    - "another-pattern"
```

No code changes are required - the system will automatically detect and handle new patterns.
