package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/calvarado2004/LlamaMux/internal/config"
	"github.com/calvarado2004/LlamaMux/internal/ocr"
	"github.com/calvarado2004/LlamaMux/internal/ollama"
	"github.com/calvarado2004/LlamaMux/internal/sd"
)

type Server struct {
	cfg    config.Config
	ollama *ollama.Client
	ocr    *ocr.Client
	sd     *sd.Client
}

func NewServer(cfg config.Config) *Server {
	return &Server{
		cfg: cfg,
		ollama: ollama.NewClient(ollama.Config{
			BaseURL:    cfg.OllamaURL,
			NumCtx:     cfg.OllamaNumCtx,
			ServerName: cfg.ServerName,
		}),
		ocr: ocr.NewClient(cfg.OCRURL),
		sd:  sd.NewClient(cfg.SDWebUIURL),
	}
}

// Router wiring
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/images/generations", s.handleImagesGenerations)
	mux.HandleFunc("/health", s.handleHealth)
}

// ---------- Utilities ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{
		"error": msg,
	})
}

// Extract a text prompt from the last message (for SD usage)
func promptFromMessages(msgs []ChatMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	last := msgs[len(msgs)-1]

	switch v := last.Content.(type) {
	case string:
		return v

	case []interface{}:
		// Look for the first text/input_text part
		for _, p := range v {
			part, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			ptype, _ := part["type"].(string)
			if ptype == "text" || ptype == "input_text" {
				if txt, ok := part["text"].(string); ok && txt != "" {
					return txt
				}
				if txt, ok := part["content"].(string); ok && txt != "" {
					return txt
				}
			}
		}

	default:
		// Fallback: stringify whatever it is
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}

	return ""
}

func toOllamaMessages(msgs []ChatMessage, ocrClient *ocr.Client) []ollama.Message {
	var out []ollama.Message

	for _, m := range msgs {
		role := m.Role
		if role == "" {
			role = "user"
		}

		switch v := m.Content.(type) {
		case string:
			out = append(out, ollama.Message{Role: role, Content: v})

		case []interface{}:
			var textParts []string
			var ocrInserts []string

			for _, p := range v {
				part, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				ptype, _ := part["type"].(string)

				if ptype == "text" || ptype == "input_text" {
					if txt, ok := part["text"].(string); ok {
						textParts = append(textParts, txt)
					} else if txt, ok := part["content"].(string); ok {
						textParts = append(textParts, txt)
					}

				} else if ptype == "image_url" || ptype == "input_image" || ptype == "image" {
					var imageURL string
					if iu, ok := part["image_url"]; ok {
						switch u := iu.(type) {
						case string:
							imageURL = u
						case map[string]interface{}:
							if ur, ok := u["url"].(string); ok {
								imageURL = ur
							}
						}
					} else if u, ok := part["url"].(string); ok {
						imageURL = u
					}

					if imageURL != "" {
						if b64, err := maybeFetchRemoteImageToB64(imageURL); err == nil && b64 != "" {
							ocrText := ocrClient.FromBase64Image(b64)
							if ocrText == "" {
								ocrText = "[OCR returned empty text]"
							}
							ocrInserts = append(ocrInserts, ocrText)
						} else {
							ocrInserts = append(ocrInserts, "[OCR could not read the image]")
						}
					}
				}
			}

			merged := strings.Join(textParts, "\n")
			if len(ocrInserts) > 0 {
				if merged != "" {
					merged += "\n\n"
				}
				merged += "[Image OCR]\n" + strings.Join(ocrInserts, "\n\n")
			}
			out = append(out, ollama.Message{Role: role, Content: merged})

		default:
			b, err := json.Marshal(v)
			if err != nil {
				out = append(out, ollama.Message{Role: role, Content: fmt.Sprint(v)})
			} else {
				out = append(out, ollama.Message{Role: role, Content: string(b)})
			}
		}
	}
	return out
}

var b64Regexp = regexp.MustCompile(`^[A-Za-z0-9+/=\r\n]+$`)

func maybeFetchRemoteImageToB64(u string) (string, error) {
	if strings.HasPrefix(u, "data:") {
		parts := strings.SplitN(u, ",", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
		return "", fmt.Errorf("bad data url")
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		resp, err := http.Get(u)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(content), nil
	}
	if b64Regexp.MatchString(strings.TrimSpace(u)) {
		return u, nil
	}
	return "", fmt.Errorf("not URL or b64")
}

func responsesToMessages(body map[string]interface{}) []ChatMessage {
	if input, ok := body["input"].([]interface{}); ok {
		var msgs []ChatMessage
		for _, t := range input {
			m, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			msgs = append(msgs, ChatMessage{
				Role:    role,
				Content: m["content"],
			})
		}
		return msgs
	}

	if msgsRaw, ok := body["messages"].([]interface{}); ok {
		var msgs []ChatMessage
		for _, t := range msgsRaw {
			m, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			msgs = append(msgs, ChatMessage{
				Role:    role,
				Content: m["content"],
			})
		}
		return msgs
	}
	return nil
}

// ---------- Handlers ----------

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	modelNames, err := s.ollama.ListModels()
	if err != nil {
		modelNames = nil
	}

	var data []ModelInfo
	for _, name := range modelNames {
		data = append(data, ModelInfo{
			ID:      name,
			Object:  "model",
			OwnedBy: s.cfg.ServerName,
		})
	}
	// add SD model
	data = append(data, ModelInfo{
		ID:      "stable-diffusion-webui-txt2img",
		Object:  "model",
		OwnedBy: s.cfg.ServerName,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   data,
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var reqBody ChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if reqBody.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required for /v1/chat/completions")
		return
	}

	// --- SPECIAL CASE: use Stable Diffusion when the "model" is the SD pseudo-model ---
	if reqBody.Model == "stable-diffusion-webui-txt2img" {
		prompt := promptFromMessages(reqBody.Messages)
		if strings.TrimSpace(prompt) == "" {
			writeError(w, http.StatusBadRequest, "prompt is empty for image generation")
			return
		}

		// You can later make size configurable; for now we just use 512x512 here.
		b64, err := s.sd.Txt2Img(prompt, 25, 7.0, "512x512")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Return as a standard chat completion with a Markdown image
		content := "![generated image](data:image/png;base64," + b64 + ")"

		resp := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl_%d", time.Now().UnixMilli()),
			"object":  "chat.completion",
			"created": NowTS(),
			"model":   reqBody.Model,
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// --- Normal path: send to Ollama as a chat completion ---
	enriched := toOllamaMessages(reqBody.Messages, s.ocr)
	model := reqBody.Model

	if reqBody.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		first := true
		for deltaText := range s.ollama.StreamChat(enriched, model) {
			delta := map[string]interface{}{"content": deltaText}
			if first {
				delta["role"] = "assistant"
				first = false
			}

			chunk := map[string]interface{}{
				"id":      fmt.Sprintf("chatcmpl_%d", time.Now().UnixMilli()),
				"object":  "chat.completion.chunk",
				"created": NowTS(),
				"model":   model,
				"choices": []interface{}{
					map[string]interface{}{
						"index":         0,
						"delta":         delta,
						"finish_reason": nil,
					},
				},
			}
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(b))
			flusher.Flush()
		}
		done := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl_%d", time.Now().UnixMilli()),
			"object":  "chat.completion.chunk",
			"created": NowTS(),
			"model":   model,
			"choices": []interface{}{
				map[string]interface{}{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
		}
		b, _ := json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	ans, err := s.ollama.CallChat(enriched, model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl_%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": NowTS(),
		"model":   model,
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": ans,
				},
				"finish_reason": "stop",
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	model, _ := body["model"].(string)
	if model == "" {
		writeError(w, http.StatusBadRequest, "model is required for /v1/responses")
		return
	}
	stream, _ := body["stream"].(bool)

	baseMsgs := responsesToMessages(body)
	enriched := toOllamaMessages(baseMsgs, s.ocr)

	if stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		var collected []string
		for deltaText := range s.ollama.StreamChat(enriched, model) {
			collected = append(collected, deltaText)
			answer := strings.Join(collected, "")

			out := map[string]interface{}{
				"id":      fmt.Sprintf("resp_%d", time.Now().UnixMilli()),
				"object":  "response",
				"created": NowTS(),
				"model":   model,
				"output": []interface{}{
					map[string]interface{}{
						"type": "message",
						"role": "assistant",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": answer,
							},
						},
					},
				},
				"usage": nil,
			}
			b, _ := json.Marshal(out)
			fmt.Fprintf(w, "data: %s\n\n", string(b))
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	ans, err := s.ollama.CallChat(enriched, model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"id":      fmt.Sprintf("resp_%d", time.Now().UnixMilli()),
		"object":  "response",
		"created": NowTS(),
		"model":   model,
		"output": []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": ans,
					},
				},
			},
		},
		"usage": nil,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleImagesGenerations(w http.ResponseWriter, r *http.Request) {
	var reqBody ImagesGenerationsRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if reqBody.Size == "" {
		reqBody.Size = "512x512"
	}
	b64, err := s.sd.Txt2Img(reqBody.Prompt, 25, 7.0, reqBody.Size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"created": NowTS(),
		"data": []interface{}{
			map[string]interface{}{
				"b64_json": b64,
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"server": "ok",
	}

	if v, err := s.ollama.HealthCheck(); err == nil {
		status["ollama"] = v
	} else {
		status["ollama"] = fmt.Sprintf("error:%v", err)
	}
	if v, err := s.sd.HealthCheck(); err == nil {
		status["sd_webui"] = v
	} else {
		status["sd_webui"] = fmt.Sprintf("error:%v", err)
	}
	if v, err := s.ocr.HealthCheck(); err == nil {
		status["ocr"] = v
	} else {
		status["ocr"] = v // unknown or error already encoded
	}

	writeJSON(w, http.StatusOK, status)
}
