package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/llm"
	"github.com/bimross/employee-factory/internal/persona"
	"github.com/bimross/employee-factory/internal/slackbot"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pl := persona.NewLoader(cfg.PersonaPath, cfg.PersonaReloadMS)
	if err := pl.Load(); err != nil {
		log.Fatalf("persona load %s: %v", cfg.PersonaPath, err)
	}
	pl.StartBackgroundReload()

	lm := llm.New(cfg)
	bot := slackbot.New(cfg, lm, pl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}

	go func() {
		log.Printf("http listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	go func() {
		if err := bot.Run(ctx); err != nil {
			log.Printf("slack bot ended: %v", err)
			cancel()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
