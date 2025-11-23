package ocr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) FromBase64Image(b64Img string) string {
	imgBytes, err := base64.StdEncoding.DecodeString(b64Img)
	if err != nil {
		return fmt.Sprintf("[OCR error: invalid base64: %v]", err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "image.png")
	if err != nil {
		return fmt.Sprintf("[OCR error: %v]", err)
	}
	if _, err := fw.Write(imgBytes); err != nil {
		return fmt.Sprintf("[OCR error: %v]", err)
	}
	w.Close()

	req, err := http.NewRequest("POST", c.BaseURL, &buf)
	if err != nil {
		return fmt.Sprintf("[OCR error: %v]", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Sprintf("[OCR error: %v]", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("[OCR error: HTTP %d: %s]", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Sprintf("[OCR error: %v]", err)
	}
	if text, ok := result["text"].(string); ok {
		return text
	}
	return ""
}

func (c *Client) HealthCheck() (string, error) {
	healthURL := c.BaseURL
	if len(healthURL) >= 4 && healthURL[len(healthURL)-4:] == "/ocr" {
		healthURL = healthURL[:len(healthURL)-4] + "/health"
	}
	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return "unknown", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "unknown", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 500 {
		return "ok", nil
	}
	return fmt.Sprintf("bad:%d", resp.StatusCode), nil
}
