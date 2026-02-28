package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

)

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := &BlueskySession{
		AccessJwt:  "access-jwt-123",
		RefreshJwt: "refresh-jwt-456",
		Handle:     "spike.bsky.social",
		DID:        "did:plc:abc123",
		Host:       "https://bsky.social",
		CreatedAt:  time.Now().Truncate(time.Second),
	}

	if err := SaveSession(original); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if loaded.AccessJwt != original.AccessJwt {
		t.Errorf("AccessJwt: got %q, want %q", loaded.AccessJwt, original.AccessJwt)
	}
	if loaded.RefreshJwt != original.RefreshJwt {
		t.Errorf("RefreshJwt: got %q, want %q", loaded.RefreshJwt, original.RefreshJwt)
	}
	if loaded.Handle != original.Handle {
		t.Errorf("Handle: got %q, want %q", loaded.Handle, original.Handle)
	}
	if loaded.DID != original.DID {
		t.Errorf("DID: got %q, want %q", loaded.DID, original.DID)
	}
	if loaded.Host != original.Host {
		t.Errorf("Host: got %q, want %q", loaded.Host, original.Host)
	}
}

func TestLoad_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := LoadSession()
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}

func TestClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	session := &BlueskySession{
		AccessJwt: "to-be-cleared",
		Handle:    "test.bsky.social",
		DID:       "did:plc:clear",
		Host:      "https://bsky.social",
		CreatedAt: time.Now(),
	}

	if err := SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := ClearSession(); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}

	_, err := LoadSession()
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession after clear, got %v", err)
	}
}

func TestFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	session := &BlueskySession{
		AccessJwt: "perm-test",
		Handle:    "test.bsky.social",
		DID:       "did:plc:perm",
		Host:      "https://bsky.social",
		CreatedAt: time.Now(),
	}

	if err := SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Check file permissions.
	path := filepath.Join(tmpDir, ".config", "signal-flow", "session.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %o, want %o", perm, 0600)
	}

	// Check directory permissions.
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}

	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf("dir permissions: got %o, want %o", dirPerm, 0700)
	}
}

func TestConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	if filepath.Base(path) != "config.json" {
		t.Fatalf("unexpected config file: %q", filepath.Base(path))
	}

	if filepath.Base(filepath.Dir(path)) != AppName {
		t.Fatalf("unexpected config dir: %q", filepath.Dir(path))
	}
}

func TestReadConfig_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	cfg, err := ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if cfg.KeyringBackend != "" {
		t.Fatalf("expected empty config, got %q", cfg.KeyringBackend)
	}
}

func TestReadConfig_JSON5(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := `{
	// allow comments + trailing commas
	keyring_backend: "file",
	}`

	if err = os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if got := strings.TrimSpace(cfg.KeyringBackend); got != "file" {
		t.Fatalf("expected keyring_backend=file, got %q", got)
	}
}
