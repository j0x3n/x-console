// Command x-console-server runs the X Console API and serves the frontend.
//
//	x-console-server            run the server (configured by XC_* env vars)
//	x-console-server gen-key    print a new XC_MASTER_KEY
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "gen-key" {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		fmt.Println(base64.StdEncoding.EncodeToString(key))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "用法: x-console-server backup <路径>")
			os.Exit(2)
		}
		source := filepath.Join(envDataDir(), "x-console.db")
		if err := store.Backup(context.Background(), source, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	level := slog.LevelInfo
	if os.Getenv("XC_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func envDataDir() string {
	if dir := os.Getenv("XC_DATA_DIR"); dir != "" {
		return dir
	}
	return "./data"
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	a, err := app.New(cfg, conn)
	if err != nil {
		return err
	}
	if err := a.Start(ctx); err != nil {
		return err
	}
	defer a.Stop()

	srv := &http.Server{Addr: cfg.Addr, Handler: a.Handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "web", cfg.WebDir != "")
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
