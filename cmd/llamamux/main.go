package main

import (
	"log"
	"net/http"
	"time"

	"github.com/calvarado2004/LlamaMux/internal/api"
	"github.com/calvarado2004/LlamaMux/internal/config"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("%s %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, duration)
	})
}

func main() {
	cfg := config.Load()
	srv := api.NewServer(cfg)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	handler := loggingMiddleware(mux)

	log.Printf("LlamaMux listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
