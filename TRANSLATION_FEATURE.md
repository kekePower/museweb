# Language Translation Feature

MuseWeb now supports automatic language translation using URL query parameters.

## Usage

Add the `lang` parameter to any URL to request content in a specific language:

```
http://localhost:8080/prompt_name?lang=LANGUAGE_CODE
```

## Examples

- **Spanish**: `http://localhost:8080/home?lang=es_ES`
- **French**: `http://localhost:8080/portfolio?lang=fr_FR`
- **German**: `http://localhost:8080/business?lang=de_DE`
- **Italian**: `http://localhost:8080/about?lang=it_IT`
- **Portuguese**: `http://localhost:8080/contact?lang=pt_BR`
- **Japanese**: `http://localhost:8080/services?lang=ja_JP`
- **Chinese**: `http://localhost:8080/products?lang=zh_CN`

## How It Works

1. **Parameter Detection**: MuseWeb extracts the `lang` parameter from the URL query string
2. **Translation Instruction**: Automatically appends "Translate everything to LANGUAGE_CODE" to the user prompt
3. **URL Preservation**: Instructs the AI to add `?lang=LANGUAGE_CODE` to all generated URLs
4. **AI Processing**: The AI model receives the translation instruction and generates content in the requested language
5. **Fallback**: If the model cannot translate to the requested language, it defaults to English

## Language Code Formats

The feature accepts various language code formats:
- **ISO 639-1**: `es`, `fr`, `de`, `it`, `pt`, `ja`, `zh`
- **ISO 639-1 + Country**: `es_ES`, `fr_FR`, `de_DE`, `en_US`, `pt_BR`
- **Language Names**: `Spanish`, `French`, `German`, `Italian`
- **Custom**: Any descriptive text like `"Spanish (Spain)"` or `"Brazilian Portuguese"`

## Debug Mode

When running MuseWeb with the `--debug` flag, you'll see additional logging:

```
🌐 Language parameter detected: es_ES
🌐 Added translation instruction: 
Translate everything to es_ES
```

## Security

- Input validation prevents injection attacks
- Language parameter is limited to 10 characters maximum
- Invalid parameters are ignored with debug logging

## Backward Compatibility

- No impact on existing URLs without the `lang` parameter
- All existing functionality remains unchanged
- Works with all AI backends (OpenAI, Ollama, etc.)

## Model Limitations

- Translation quality depends on the AI model's language capabilities
- Some models may have better support for certain languages
- If a model cannot translate to the requested language, it will default to English
- Complex language codes might not be understood by all models

## Language Context Preservation

**Key Feature**: When a language parameter is present, all generated URLs will automatically include the same language parameter.

### Example Behavior

**Input URL**: `http://localhost:8080/home?lang=es_ES`

**Generated Links Will Include**:
- `<a href="/about?lang=es_ES">Acerca de</a>`
- `<a href="/contact?lang=es_ES">Contacto</a>`
- `<a href="/services?lang=es_ES">Servicios</a>`

**Instead of**:
- `<a href="/about">Acerca de</a>` ❌
- `<a href="/contact">Contacto</a>` ❌
- `<a href="/services">Servicios</a>` ❌

This ensures users stay in their chosen language when navigating through the generated website.

## Examples in Different Languages

### Spanish Website
```
http://localhost:8080/home?lang=es_ES
```
Result: Complete HTML page generated in Spanish with all links preserving `?lang=es_ES`

### French Portfolio
```
http://localhost:8080/portfolio?lang=fr_FR
```
Result: Portfolio page with French content and navigation links including `?lang=fr_FR`

### German Business Page
```
http://localhost:8080/business?lang=de_DE
```
Result: Business-focused page in German with all URLs containing `?lang=de_DE`

## Technical Implementation

The translation feature works by:
1. Extracting the `lang` parameter from `r.URL.Query().Get("lang")`
2. Validating and sanitizing the input
3. Appending a comprehensive translation instruction to the user prompt:
   - "Translate everything to LANGUAGE_CODE"
   - "IMPORTANT: Add ?lang=LANGUAGE_CODE to all generated URLs to preserve the language context."
4. Letting the AI model handle both translation and URL modification naturally

This approach leverages the AI model's built-in multilingual capabilities and instruction-following abilities rather than requiring separate translation services or post-processing URL modification.
