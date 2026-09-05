package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/buildinfo"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/httpapi"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/llamacpp"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/ollama"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	checkConfig := flag.Bool("check-config", false, "validate configuration and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Quantum Runtime %s (%s, %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Quantum Runtime configuration error: %v\n", err)
		os.Exit(2)
	}
	if *checkConfig {
		fmt.Printf("configuration valid: listen=%s backend=%s ollama=%s llama_cpp=%s mutation=%t auth=%t\n",
			cfg.ListenAddress,
			cfg.Backend,
			cfg.UpstreamURL.Redacted(),
			cfg.LlamaCPPURL.Redacted(),
			cfg.AllowModelMutation,
			cfg.AuthToken != "",
		)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var backend httpapi.Upstream
	switch cfg.Backend {
	case "llama.cpp":
		backend = llamacpp.New(cfg.LlamaCPPURL, buildinfo.Version, cfg.LlamaCPPModel, cfg.LlamaCPPAPIKey)
	default:
		backend = ollama.NewProxy(cfg.UpstreamURL, buildinfo.Version)
	}
	api := httpapi.New(cfg, backend, httpapi.BuildInfo{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		BuildDate: buildinfo.BuildDate,
	}, logger)

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("Quantum Runtime starting",
			"version", buildinfo.Version,
			"listen", cfg.ListenAddress,
			"backend", backend.Descriptor().Kind,
			"model_mutation", cfg.AllowModelMutation,
		)
		serveErrors <- server.ListenAndServe()
	}()

	select {
	case <-rootContext.Done():
		logger.Info("Quantum Runtime stopping")
	case err := <-serveErrors:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Quantum Runtime stopped unexpectedly", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("Quantum Runtime shutdown failed", "error", err)
		os.Exit(1)
	}

	if err := <-serveErrors; err != nil && err != http.ErrServerClosed {
		logger.Error("Quantum Runtime server exit failed", "error", err)
		os.Exit(1)
	}
	logger.Info("Quantum Runtime stopped")
}
