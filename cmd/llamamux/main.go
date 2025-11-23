package main

import (
	"log"
	"net/http"

	"github.com/calvarado2004/LlamaMux/internal/api"
	"github.com/calvarado2004/LlamaMux/internal/config"
)

func main() {
	cfg := config.Load()
	srv := api.NewServer(cfg)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	log.Printf("LlamaMux listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

