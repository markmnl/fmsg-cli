package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The exchanged JWT for an environment-supplied API key is cached on disk so
// that successive CLI invocations (each a fresh process) don't re-exchange
// the key on every call. The cache holds only the short-lived token, never
// the API key, and lives in the user CACHE dir — separate from auth.json, so
// `login` credentials are never touched by env-key use. Keyed by
// sha256(apiURL, apiKey): a different key or host never sees another's token.
// Set FMSG_NO_TOKEN_CACHE=1 to disable.

const envNoTokenCache = "FMSG_NO_TOKEN_CACHE"

// tokenCachePath returns the cache file for (apiURL, apiKey), or "" when
// caching is disabled or no cache dir is available.
func tokenCachePath(apiURL, apiKey string) string {
	if os.Getenv(envNoTokenCache) != "" {
		return ""
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(apiURL + "\x00" + apiKey))
	return filepath.Join(base, "fmsg", "tokens", hex.EncodeToString(sum[:16])+".json")
}

// loadCachedToken reads a cached token; a missing or unreadable cache is
// simply a miss.
func loadCachedToken(path string) (Credentials, bool) {
	if path == "" {
		return Credentials{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, false
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil || creds.AccessToken == "" {
		return Credentials{}, false
	}
	return creds, true
}

// saveCachedToken writes the token with 0600 permissions via temp+rename so a
// concurrent invocation never reads a partial file. Failures are ignored by
// callers: the cache is an optimisation.
func saveCachedToken(path string, creds Credentials) error {
	if path == "" {
		return errors.New("token cache disabled")
	}
	creds.APIKey = "" // never persist the key itself
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tok-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("installing token cache: %w", err)
	}
	return nil
}
