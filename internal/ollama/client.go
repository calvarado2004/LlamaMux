package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Config struct {
	BaseURL   string
	NumCtx    int
	ServerName string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Options struct {
	NumCtx int `json:"num_ctx"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  Options   `json:"options"`
}

type TagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type Client struct {
	cfg    Config
	http   *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) chatRequest(messages []Message, model string, numCtx int, stream bool) (*http.Response, error) {
	payload := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
		Options:  Options{NumCtx: numCtx},
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", c.cfg.BaseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}

// CallChat – non-streaming, with context fallback
func (c *Client) CallChat(messages []Message, model string) (string, error) {
	if model == "" {
		return "", fmt.Errorf("model is required")
	}

	tries := []int{c.cfg.NumCtx}
	for _, v := range []int{65536, 32768, 8192} {
		if v != c.cfg.NumCtx {
			tries = append(tries, v)
		}
	}

	var lastErr error
	for i, ctx := range tries {
		resp, err := c.chatRequest(messages, model, ctx, false)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue
		}

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			lastErr = err
			continue
		}

		msg, _ := data["message"].(map[string]interface{})
		content, _ := msg["content"].(string)
		if content == "" {
			content, _ = data["response"].(string)
		}

		if i > 0 {
			content = fmt.Sprintf("[ctx fallback to %d]\n%s", ctx, content)
		}
		return content, nil
	}
	return "", fmt.Errorf("Ollama error after ctx fallbacks: %v", lastErr)
}

// StreamChat returns a channel of delta strings
func (c *Client) StreamChat(messages []Message, model string) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)

		resp, err := c.chatRequest(messages, model, c.cfg.NumCtx, true)
		if err != nil {
			ch <- fmt.Sprintf("\n[Ollama streaming error: %v]\n", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			ch <- fmt.Sprintf("\n[Ollama streaming error: HTTP %d: %s]\n", resp.StatusCode, string(body))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			line = trimDataPrefix(line)
			if line == "" {
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				continue
			}

			if done, ok := data["done"].(bool); ok && done {
				break
			}

			var delta string
			if msg, ok := data["message"].(map[string]interface{}); ok {
				if c, ok := msg["content"].(string); ok {
					delta = c
				}
			}
			if delta == "" {
				if c, ok := data["response"].(string); ok {
					delta = c
				}
			}
			if delta != "" {
				ch <- delta
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- fmt.Sprintf("\n[Ollama streaming error: %v]\n", err)
		}
	}()
	return ch
}

func trimDataPrefix(line string) string {
	line = string(bytes.TrimSpace([]byte(line)))
	if line == "" {
		return ""
	}
	if len(line) >= 5 && line[:5] == "data:" {
		line = string(bytes.TrimSpace([]byte(line[5:])))
	}
	return line
}

// ListModels wraps /api/tags
func (c *Client) ListModels() ([]string, error) {
	req, err := http.NewRequest("GET", c.cfg.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tags TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range tags.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out, nil
}

// HealthCheck simple GET on root
func (c *Client) HealthCheck() (string, error) {
	req, err := http.NewRequest("GET", c.cfg.BaseURL+"/", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 500 {
		return "ok", nil
	}
	return fmt.Sprintf("bad:%d", resp.StatusCode), nil
}

