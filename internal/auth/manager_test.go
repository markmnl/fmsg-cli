package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoginExchangesAPIKeyAndStoresCredentials(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	jwt := testJWT("@user_bot@example.com")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fmsg/token" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer fmsgk_id_secret"; got != want {
			t.Fatalf("Authorization: want %q, got %q", want, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": jwt,
			"token_type":   "Bearer",
			"expires_in":   43200,
		})
	}))
	defer srv.Close()

	manager := NewManager(srv.URL)
	manager.now = func() time.Time { return now }

	tok, err := manager.Login(context.Background(), "fmsgk_id_secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if tok.AccessToken != jwt {
		t.Fatalf("access token not returned")
	}
	if tok.User != "@user_bot@example.com" {
		t.Fatalf("user: want sub account, got %q", tok.User)
	}
	if !tok.ExpiresAt.Equal(now.Add(12 * time.Hour)) {
		t.Fatalf("expires_at: want expires_in fallback, got %s", tok.ExpiresAt)
	}

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.Version != 2 || creds.APIKey != "fmsgk_id_secret" || creds.APIURL != srv.URL {
		t.Fatalf("stored credentials not updated correctly: %+v", creds)
	}

	path, err := storePath()
	if err != nil {
		t.Fatalf("storePath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions: want 0600, got %o", info.Mode().Perm())
	}
}

func TestAccessTokenUsesCacheAndRefreshesNearExpiry(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": testJWT("@user_bot@example.com"),
			"token_type":   "Bearer",
			"expires_at":   now.Add(12 * time.Hour).Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	if err := Save(Credentials{
		Version:     2,
		APIKey:      "fmsgk_id_secret",
		AccessToken: testJWT("@cached@example.com"),
		TokenType:   "Bearer",
		ExpiresAt:   now.Add(6 * time.Hour),
		User:        "@cached@example.com",
		APIURL:      srv.URL,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manager := NewManager(srv.URL)
	manager.now = func() time.Time { return now }
	token, err := manager.AccessToken(context.Background(), false)
	if err != nil {
		t.Fatalf("AccessToken cached: %v", err)
	}
	if token != testJWT("@cached@example.com") || count != 0 {
		t.Fatalf("expected cached token without exchange, token=%q count=%d", token, count)
	}

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	creds.ExpiresAt = now.Add(4 * time.Minute)
	if err := Save(creds); err != nil {
		t.Fatalf("Save near-expiry: %v", err)
	}

	if _, err := manager.AccessToken(context.Background(), false); err != nil {
		t.Fatalf("AccessToken refresh: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one refresh exchange, got %d", count)
	}
}

func TestAccessTokenInvalidatesCacheWhenAPIURLChanges(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": testJWT("@user_bot@example.com"),
			"token_type":   "Bearer",
			"expires_at":   now.Add(12 * time.Hour).Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	if err := Save(Credentials{
		Version:     2,
		APIKey:      "fmsgk_id_secret",
		AccessToken: testJWT("@cached@example.com"),
		TokenType:   "Bearer",
		ExpiresAt:   now.Add(6 * time.Hour),
		User:        "@cached@example.com",
		APIURL:      "http://old.example",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manager := NewManager(srv.URL)
	manager.now = func() time.Time { return now }
	if _, err := manager.AccessToken(context.Background(), false); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected cache invalidation exchange, got %d", count)
	}
}

func TestEnvAPIKeyOverridesStoredCredentialsWithoutPersisting(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "fmsgk_env_secret")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer fmsgk_env_secret"; got != want {
			t.Fatalf("Authorization: want %q, got %q", want, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": testJWT("@env_bot@example.com"),
			"token_type":   "Bearer",
			"expires_at":   now.Add(12 * time.Hour).Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	manager := NewManager(srv.URL)
	manager.now = func() time.Time { return now }
	if _, err := manager.AccessToken(context.Background(), false); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}

	path, err := storePath()
	if err != nil {
		t.Fatalf("storePath: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("env auth should not persist credentials, stat err=%v", err)
	}
}

func TestRejectedAPIKeyErrorDoesNotIncludeSecret(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	manager := NewManager(srv.URL)
	_, err := manager.Login(context.Background(), "fmsgk_id_supersecret")
	if err == nil {
		t.Fatal("expected login error")
	}
	if strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), "fmsgk_id") {
		t.Fatalf("error leaked API key: %v", err)
	}
}

func TestLoginRejectsAddressInput(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	manager := NewManager(srv.URL)
	_, err := manager.Login(context.Background(), "@user@example.com")
	if err == nil {
		t.Fatal("expected address input rejection")
	}
	if called {
		t.Fatal("server should not be called for address-shaped login input")
	}
}

func TestLoginJWTStoresUserCredentials(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	jwt := testJWTWithClaims(map[string]any{
		"sub":          "idp-user-123",
		"fmsg_address": "@alice@example.com",
		"exp":          now.Add(time.Hour).Unix(),
	})

	manager := NewManager("http://api.example")
	manager.now = func() time.Time { return now }

	tok, err := manager.LoginJWT(jwt, "")
	if err != nil {
		t.Fatalf("LoginJWT: %v", err)
	}
	if tok.User != "@alice@example.com" {
		t.Fatalf("user: want address claim, got %q", tok.User)
	}

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.AuthType != "jwt" || creds.APIKey != "" || creds.AccessToken != jwt {
		t.Fatalf("stored JWT credentials not correct: %+v", creds)
	}

	token, err := manager.AccessToken(context.Background(), false)
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if token != jwt {
		t.Fatalf("stored JWT token: want original token")
	}
}

func TestLoginJWTUsesExplicitAddressWhenClaimMissing(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	jwt := testJWTWithClaims(map[string]any{
		"sub": "idp-user-123",
		"exp": now.Add(time.Hour).Unix(),
	})

	manager := NewManager("http://api.example")
	manager.now = func() time.Time { return now }

	tok, err := manager.LoginJWT(jwt, "@alice@example.com")
	if err != nil {
		t.Fatalf("LoginJWT: %v", err)
	}
	if tok.User != "@alice@example.com" {
		t.Fatalf("user: want explicit address, got %q", tok.User)
	}
}

func TestStoredJWTCannotRefreshAndExpires(t *testing.T) {
	setTestConfigDir(t)
	t.Setenv("FMSG_API_KEY", "")

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	jwt := testJWTWithClaims(map[string]any{
		"sub":          "idp-user-123",
		"fmsg_address": "@alice@example.com",
		"exp":          now.Add(time.Hour).Unix(),
	})

	if err := Save(Credentials{
		Version:     2,
		AuthType:    "jwt",
		AccessToken: jwt,
		TokenType:   "Bearer",
		ExpiresAt:   now.Add(time.Hour),
		User:        "@alice@example.com",
		APIURL:      "http://api.example",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manager := NewManager("http://api.example")
	manager.now = func() time.Time { return now }
	if _, err := manager.AccessToken(context.Background(), true); err == nil {
		t.Fatal("expected force refresh error for stored JWT")
	}

	manager.now = func() time.Time { return now.Add(56 * time.Minute) }
	if _, err := manager.AccessToken(context.Background(), false); err == nil {
		t.Fatal("expected near-expiry JWT error")
	}
}

func setTestConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	return filepath.Join(dir, "fmsg")
}

func testJWT(sub string) string {
	return testJWTWithClaims(map[string]any{"sub": sub})
}

func testJWTWithClaims(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".signature"
}
