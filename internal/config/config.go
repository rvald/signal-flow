// Package config manages local session storage for Signal-Flow CLI.
// Sessions are stored as JSON files in ~/.config/signal-flow/.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"github.com/yosuke-furukawa/json5/encoding/json5"
)

// ErrNoSession is returned when no session file exists.
var ErrNoSession = errors.New("no session found — run 'signal-flow login' first")

type File struct {
	KeyringBackend  string            `json:"keyring_backend,omitempty"`
	DefaultTimezone string            `json:"default_timezone,omitempty"`
	AccountAliases  map[string]string `json:"account_aliases,omitempty"`
	AccountClients  map[string]string `json:"account_clients,omitempty"`
	ClientDomains   map[string]string `json:"client_domains,omitempty"`
}

// BlueskySession holds the authenticated session data from ServerCreateSession.
type BlueskySession struct {
	AccessJwt  string    `json:"access_jwt"`
	RefreshJwt string    `json:"refresh_jwt"`
	Handle     string    `json:"handle"`
	DID        string    `json:"did"`
	Host       string    `json:"host"`
	CreatedAt  time.Time `json:"created_at"`
}

// configDir returns ~/.config/signal-flow, creating it if needed.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "signal-flow")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	return dir, nil
}

// sessionPath returns the full path to the session file.
func sessionPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// SaveSession writes the Bluesky session to disk.
func SaveSession(session *BlueskySession) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	return nil
}

// LoadSession reads the Bluesky session from disk.
// Returns ErrNoSession if the file does not exist.
func LoadSession() (*BlueskySession, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSession
		}
		return nil, fmt.Errorf("read session: %w", err)
	}

	var session BlueskySession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &session, nil
}

// ClearSession removes the session file.
func ClearSession() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session: %w", err)
	}

	return nil
}

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func WriteConfig(cfg File) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config json: %w", err)
	}

	b = append(b, '\n')

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}

	return nil
}

func ConfigExists() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}

		return false, fmt.Errorf("stat config: %w", statErr)
	}

	return true, nil
}

func ReadConfig() (File, error) {
	path, err := ConfigPath()
	if err != nil {
		return File{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec // config file path
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}

		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json5.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}
