// Package auth handles credential storage on disk.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Credentials holds the stored authentication state.
type Credentials struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      string    `json:"user"`
}

// storePath returns the path to auth.json, creating parent directories as needed.
func storePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating config directory: %w", err)
	}
	dir := filepath.Join(configDir, "fmsg")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}
	return filepath.Join(dir, "auth.json"), nil
}

// Save writes credentials to disk with 0600 permissions.
func Save(creds Credentials) error {
	path, err := storePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

// Load reads stored credentials from disk.
func Load() (Credentials, error) {
	path, err := storePath()
	if err != nil {
		return Credentials{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, fmt.Errorf("no stored credentials found")
		}
		return Credentials{}, fmt.Errorf("reading credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("decoding credentials: %w", err)
	}
	return creds, nil
}

// LoadValid loads stored credentials and returns an error if they are missing
// or expired. The error message instructs the user to run fmsg login.
func LoadValid() (Credentials, error) {
	creds, err := Load()
	if err != nil {
		return Credentials{}, fmt.Errorf("you must login first using: fmsg login")
	}
	if time.Now().After(creds.ExpiresAt) {
		return Credentials{}, fmt.Errorf("token expired — you must login first using: fmsg login")
	}
	return creds, nil
}
