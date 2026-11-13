package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"pg-admin-pro/internal/auth"
	"pg-admin-pro/internal/backup"
	"pg-admin-pro/internal/config"
	"pg-admin-pro/internal/cryptoutil"
	"pg-admin-pro/internal/server"
	"pg-admin-pro/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	encKey, err := cryptoutil.LoadOrCreateKey(cfg.DataDir, cfg.ProfileEncKey)
	if err != nil {
		log.Fatalf("load master encryption key: %v", err)
	}
	profiles, err := store.NewProfileStore(cfg.DataDir, encKey)
	if err != nil {
		log.Fatalf("load saved server profiles: %v", err)
	}
	snippets, err := store.NewSnippetStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("load saved sql snippets: %v", err)
	}

	sessions := auth.NewManager(cfg.SessionTTL)
	defer sessions.Close()

	backups := backup.NewManagerWithKey(cfg, encKey)
	backups.StartScheduler(rootCtx)

	srv := server.NewWithStores(cfg, sessions, backups, profiles, snippets)
	httpServer := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("pg-admin-pro listening on %s (static=%s, backups=%s)", cfg.AppAddr, cfg.StaticDir, cfg.BackupDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-rootCtx.Done()
	log.Println("shutting down pg-admin-pro...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
