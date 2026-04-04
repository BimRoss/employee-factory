package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bimross/employee-factory/internal/opsproxy"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := opsproxy.LoadProxyConfigFromEnv()
	if err != nil {
		log.Fatalf("proxy config: %v", err)
	}

	srvImpl, err := opsproxy.NewProxyServer(cfg)
	if err != nil {
		log.Fatalf("proxy init: %v", err)
	}
	defer func() {
		if err := srvImpl.Close(); err != nil {
			log.Printf("proxy close: %v", err)
		}
	}()

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: srvImpl.Handler(),
	}

	go func() {
		log.Printf("ops proxy listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ops proxy: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
