package config

import (
	"os"
	"strconv"
)

type Config struct {
	OllamaURL    string
	SDWebUIURL   string
	OCRURL       string
	ServerName   string
	OllamaNumCtx int
	ListenAddr   string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func Load() Config {
	return Config{
		OllamaURL:    getenv("OLLAMA_URL", "http://192.168.1.88:11434"),
		SDWebUIURL:   getenv("SD_WEBUI_URL", "http://192.168.122.1:7860"),
		OCRURL:       getenv("OCR_URL", "http://192.168.122.1:5055/ocr"),
		ServerName:   getenv("SERVER_NAME", "LlamaMux"),
		OllamaNumCtx: getEnvInt("OLLAMA_NUM_CTX", 8192),
		ListenAddr:   getenv("LLAMAMUX_ADDR", ":8001"),
	}
}

