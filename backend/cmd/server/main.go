// Command x-console-server runs the X Console API and serves the frontend.
//
//	x-console-server            run the server (configured by XC_* env vars)
//	x-console-server gen-key    print a new XC_MASTER_KEY
//	x-console-server pairing-code --name <host> --kind server
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/events"
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
	if len(os.Args) > 1 && os.Args[1] == "pairing-code" {
		if err := runPairingCode(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "reset-vault-password" {
		if len(os.Args) != 2 {
			fmt.Fprintln(os.Stderr, "用法: x-console-server reset-vault-password")
			os.Exit(2)
		}
		if err := resetVaultPassword(); err != nil {
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

func runPairingCode(args []string) error {
	fs := flag.NewFlagSet("pairing-code", flag.ContinueOnError)
	name := fs.String("name", "", "host name")
	kind := fs.String("kind", "server", "agent kind")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *name == "" {
		return errors.New("用法: x-console-server pairing-code --name <名称> --kind server")
	}
	code, err := pairingCode(context.Background(), filepath.Join(envDataDir(), "x-console.db"), *name, *kind)
	if err != nil {
		return err
	}
	fmt.Println(code)
	return nil
}

func pairingCode(ctx context.Context, path, name, kind string) (string, error) {
	conn, err := store.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	log := audit.New(conn)
	hub := agenthub.New(conn, events.NewBus(), log)
	code, _, err := hub.CreatePairingCode(audit.WithActor(ctx, "system:deploy"), name, kind)
	return code, err
}

func resetVaultPassword() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("请在终端运行重置命令")
	}
	fmt.Fprint(os.Stderr, "新隐藏密码（至少 6 位）：")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	password := strings.TrimSuffix(string(raw), "\r")
	if len(password) < 6 {
		return errors.New("隐藏密码至少 6 位")
	}
	fmt.Fprint(os.Stderr, "再次输入新隐藏密码：")
	repeat, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	if password != strings.TrimSuffix(string(repeat), "\r") {
		return errors.New("两次密码不一致")
	}
	conn, err := store.Open(context.Background(), cfg.DBPath())
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := auth.ResetVaultPassword(context.Background(), conn, password); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "隐藏密码已重置，隐藏内容保留")
	return nil
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
