# LlamaMux

**LlamaMux** is a lightweight Golang gateway that exposes an **OpenAI‑compatible API** in front of your **local AI stack**:

- Ollama (LLM inference)
- OCR backend (image text extraction)
- Stable Diffusion WebUI (image generation)
- *Future:* Retrieval-Augmented Generation (RAG)

It is designed to act as a local, self‑hosted, multimodal LLM orchestration layer.

Author: Carlos Alvarado

---

## Features

###  OpenAI-Compatible Endpoints
LlamaMux provides drop‑in replacements for common OpenAI APIs:

| Endpoint | Description |
|----------|-------------|
| `GET /v1/models` | Lists Ollama models (+ SD pseudo-model) |
| `POST /v1/chat/completions` | Chat API (streaming + non-stream) |
| `POST /v1/responses` | OpenAI Responses API shim |
| `POST /v1/images/generations` | Image generation via Stable Diffusion |
| `GET /health` | Health checks for Ollama, SD, OCR |

### 🔹 Multimodal Support (OCR)
- Supports message parts such as:  
  - `{ "type": "text" }`  
  - `{ "type": "image_url" }`
- Images are downloaded or decoded from:
  - URL (`http(s)://`)
  - `data:image/...;base64,...`
  - Raw base64
- Extracted text is automatically appended to the user prompt.

###  Context Fallback Logic for Ollama
If a large context window fails, LlamaMux retries automatically using smaller context sizes (e.g., 65k → 32k → 8k).

###  Simple, Modular Golang Architecture
```
cmd/llamamux/      → Main server entrypoint
internal/config/   → Environment config
internal/ollama/   → Ollama client + streaming
internal/sd/       → Stable Diffusion client
internal/ocr/      → OCR client
internal/api/      → HTTP handlers + API schemas
internal/rag/      → (future) retrieval pipeline
```

---

##  Getting Started

### 1. Install Go
Requires **Go 1.21+** (recommended 1.22 or higher).

### 2. Clone the repo
```bash
git clone https://github.com/calvarado2004/LlamaMux.git
cd LlamaMux
```

### 3. Build
```bash
go mod tidy
go build ./cmd/llamamux
```

### 4. Run
```bash
./llamamux
```

Default listen address:  
```
http://localhost:8001
```

---

##  Configuration

Environment variables:

| Variable | Default | Description |
|---------|---------|-------------|
| `OLLAMA_URL` | `http://localhost:11434` | Ollama API |
| `SD_WEBUI_URL` | `http://localhost:7860` | Stable Diffusion WebUI |
| `OCR_URL` | `http://localhost:5055/ocr` | OCR service |
| `OLLAMA_NUM_CTX` | `8192` | Preferred context window |
| `SERVER_NAME` | `LlamaMux` | Identity exposed in `/v1/models` |
| `LLAMAMUX_ADDR` | `:8001` | Listen address |

Example:
```bash
export OLLAMA_URL="http://127.0.0.1:11434"
export SD_WEBUI_URL="http://127.0.0.1:7860"
export OCR_URL="http://127.0.0.1:5055/ocr"
export LLAMAMUX_ADDR="0.0.0.0:8001"

./llamamux
```

---

## Example: Chat Completion

```json
POST /v1/chat/completions
{
  "model": "gpt-oss-20b",
  "messages": [
    { "role": "user", "content": "Hello LlamaMux!" }
  ],
  "stream": false
}
```

---

## Example: Multimodal Input

```json
{
  "model": "gpt-oss-20b",
  "messages": [
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "Extract the text from this image:" },
        {
          "type": "image_url",
          "image_url": { "url": "https://example.com/screenshot.png" }
        }
      ]
    }
  ]
}
```

LlamaMux will:
1. Download the image  
2. OCR the contents  
3. Append:

```
[Image OCR]
<recognized text>
```

---

## Example: Image Generation

```json
POST /v1/images/generations
{
  "prompt": "a glowing futuristic data center",
  "size": "768x512"
}
```

Returns a base64‑encoded PNG.

---

##  Future Roadmap

-  **RAG support** (embeddings, vector store, hybrid search)
-  **Better structured logging**
-  **Optional API key authentication**
-  **Inference tool plugins**

---

## License

Apache 2.0

---

## Contributing

PRs, issues, and feature suggestions are welcome!  
This project began as a personal experiment to unify a local multimodal LLM stack—feel free to build on top of it.
