package sd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"strings"
)

type Client struct {
	BaseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		http:    &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) Txt2Img(prompt string, steps int, cfgScale float64, size string) (string, error) {
	wid, hei := 512, 512
	if size != "" {
		parts := strings.Split(strings.ToLower(size), "x")
		if len(parts) == 2 {
			if w, err := strconv.Atoi(parts[0]); err == nil && w > 0 {
				wid = w
			}
			if h, err := strconv.Atoi(parts[1]); err == nil && h > 0 {
				hei = h
			}
		}
	}

	payload := map[string]interface{}{
		"prompt":       prompt,
		"steps":        steps,
		"cfg_scale":    cfgScale,
		"width":        wid,
		"height":       hei,
		"sampler_name": "Euler a",
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", c.BaseURL+"/sdapi/v1/txt2img", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Stable Diffusion error: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	images, ok := data["images"].([]interface{})
	if !ok || len(images) == 0 {
		return "", fmt.Errorf("No image returned by Stable Diffusion")
	}
	first, _ := images[0].(string)
	return first, nil
}

func (c *Client) HealthCheck() (string, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/sdapi/v1/sd-models", nil)
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
