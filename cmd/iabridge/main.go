package main

import (
	"fmt"
	"log"
	"net/http"

	"iabridge/internal/config"
	"iabridge/internal/downloads"
	"iabridge/internal/handler"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	store, err := downloads.New(cfg.DataDir, cfg.IABin)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	mux := http.NewServeMux()
	handler.Register(mux, cfg, store)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("iabridge listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
