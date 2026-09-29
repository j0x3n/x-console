// Package config reads server settings from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

// Config is the runtime configuration of x-console-server.
type Config struct {
	// Addr is the listen address, XC_ADDR, default 127.0.0.1:8080.
	Addr string
	// DataDir holds the SQLite database and uploaded files, XC_DATA_DIR, default ./data.
	DataDir string
	// WebDir is the built frontend (web/dist). Empty means API only. XC_WEB_DIR.
	WebDir string
	// PublicURL is the external URL, used in links sent by notifications. XC_PUBLIC_URL.
	PublicURL string
	// MasterKey encrypts stored secrets. XC_MASTER_KEY, base64 of 32 bytes.
	MasterKey []byte
	// Dev relaxes cookie security for plain-HTTP local development. XC_DEV=1.
	Dev bool
	// Location is the user's time zone for reminders, habits and "today". XC_TZ, default Asia/Shanghai.
	Location *time.Location
}

// DBPath is the SQLite database file.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "x-console.db") }

// FilesDir is where uploaded files live: <DataDir>/files/<module>/... (B24).
func (c Config) FilesDir() string { return filepath.Join(c.DataDir, "files") }

// BackupsDir holds backup packages made on this machine (B25).
func (c Config) BackupsDir() string { return filepath.Join(c.DataDir, "backups") }

// RestoreDir is where a backup waits to be applied when the server restarts (B25).
func (c Config) RestoreDir() string { return filepath.Join(c.DataDir, "restore-tmp") }

// FilesCacheDir keeps local copies of files while the site stores them in S3.
func (c Config) FilesCacheDir() string { return filepath.Join(c.DataDir, "files-cache") }

// TmpDir holds files that are still being received. It is emptied at start.
func (c Config) TmpDir() string { return filepath.Join(c.DataDir, "tmp") }

// FromEnv loads the configuration and validates it.
func FromEnv() (Config, error) {
	c := Config{
		Addr:      env("XC_ADDR", "127.0.0.1:8080"),
		DataDir:   env("XC_DATA_DIR", "./data"),
		WebDir:    os.Getenv("XC_WEB_DIR"),
		PublicURL: strings.TrimRight(os.Getenv("XC_PUBLIC_URL"), "/"),
		Dev:       os.Getenv("XC_DEV") == "1",
	}
	loc, err := time.LoadLocation(env("XC_TZ", "Asia/Shanghai"))
	if err != nil {
		return c, fmt.Errorf("XC_TZ: %w", err)
	}
	c.Location = loc
	raw := os.Getenv("XC_MASTER_KEY")
	if raw == "" {
		return c, errors.New("XC_MASTER_KEY is required (generate one with: openssl rand -base64 32)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return c, fmt.Errorf("XC_MASTER_KEY must be base64 of 32 bytes")
	}
	c.MasterKey = key
	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
