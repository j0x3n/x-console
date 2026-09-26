// Package config stores the agent's pairing result on disk.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is saved after pairing.
type Config struct {
	Server  string `json:"server"` // for example https://console.example.com
	AgentID string `json:"agentId"`
	Token   string `json:"token"`
	// Coding holds the M4 runner settings (repo roots, executor paths).
	Coding json.RawMessage `json:"coding,omitempty"`
}

// DefaultPath is <user config dir>/x-console-agent/config.json.
// On Windows that is %AppData%\x-console-agent\config.json; on Linux when run
// as root under systemd, use --config /etc/x-console-agent/config.json.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "x-console-agent", "config.json")
}

// Load reads the config file.
func Load(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, errors.New("agent is not paired yet; run: x-console-agent pair --server <url> --code <code>")
		}
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}

// Save writes the config file with owner-only permissions.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
