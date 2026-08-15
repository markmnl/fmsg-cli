package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/markmnl/fmsg-cli/internal/config"
)

const (
	credentialsVersion = 2
	refreshBefore      = 5 * time.Minute
	authTypeAPIKey     = "api_key"
	authTypeJWT        = "jwt"
)

// Token is a short-lived access token returned by fmsg-webapi.
type Token struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	User        string
}

// Manager exchanges opaque API keys for short-lived JWTs and caches them.
type Manager struct {
	APIURL string
	HTTP   *http.Client

	mu       sync.Mutex
	envCache Credentials
	now      func() time.Time
}

// NewManager returns a Manager for the configured fmsg-webapi base URL.
func NewManager(apiURL string) *Manager {
	return &Manager{
		APIURL: strings.TrimRight(apiURL, "/"),
		HTTP:   &http.Client{},
		now:    time.Now,
	}
}

// AccessToken returns a valid access token. If forceRefresh is true, the API key
// is exchanged even when a cached JWT is still fresh.
func (m *Manager) AccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	tok, err := m.Token(ctx, forceRefresh)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// Token returns the current access token and metadata.
func (m *Manager) Token(ctx context.Context, forceRefresh bool) (Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if apiKey := strings.TrimSpace(config.GetAPIKey()); apiKey != "" {
		return m.tokenFromEnv(ctx, apiKey, forceRefresh)
	}

	creds, err := Load()
	if err != nil {
		return Token{}, fmt.Errorf("missing credentials; run fmsg login with an API key or user JWT, or set FMSG_API_KEY")
	}
	if strings.TrimSpace(creds.APIKey) == "" {
		return m.tokenFromStoredJWT(creds, forceRefresh)
	}
	if !forceRefresh && m.cachedTokenValid(creds) {
		return tokenFromCredentials(creds), nil
	}

	tok, err := m.exchange(ctx, creds.APIKey)
	if err != nil {
		return Token{}, err
	}
	creds.Version = credentialsVersion
	creds.AuthType = authTypeAPIKey
	creds.AccessToken = tok.AccessToken
	creds.TokenType = tok.TokenType
	creds.ExpiresAt = tok.ExpiresAt
	creds.User = tok.User
	creds.APIURL = m.APIURL
	if err := Save(creds); err != nil {
		return Token{}, fmt.Errorf("saving refreshed credentials: %w", err)
	}
	return tok, nil
}

// User returns the fmsg address from the current access token's sub claim.
func (m *Manager) User(ctx context.Context) (string, error) {
	tok, err := m.Token(ctx, false)
	if err != nil {
		return "", err
	}
	if tok.User == "" {
		return "", fmt.Errorf("access token did not contain an fmsg address")
	}
	return tok.User, nil
}

// Login exchanges and stores an API key supplied by the user.
func (m *Manager) Login(ctx context.Context, apiKey string) (Token, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return Token{}, fmt.Errorf("API key must not be empty")
	}
	if !strings.HasPrefix(apiKey, "fmsgk_") {
		return Token{}, fmt.Errorf("API-key login expects a key starting with fmsgk_; for main-account login, pass a user JWT")
	}

	tok, err := m.exchange(ctx, apiKey)
	if err != nil {
		return Token{}, err
	}
	creds := Credentials{
		Version:     credentialsVersion,
		AuthType:    authTypeAPIKey,
		APIKey:      apiKey,
		AccessToken: tok.AccessToken,
		TokenType:   tok.TokenType,
		ExpiresAt:   tok.ExpiresAt,
		User:        tok.User,
		APIURL:      m.APIURL,
	}
	if err := Save(creds); err != nil {
		return Token{}, fmt.Errorf("saving credentials: %w", err)
	}
	return tok, nil
}

// LoginJWT stores a user JWT issued by the deployment's identity provider.
func (m *Manager) LoginJWT(token, address string) (Token, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Token{}, fmt.Errorf("JWT must not be empty")
	}

	claims, err := jwtClaims(token)
	if err != nil {
		return Token{}, err
	}
	if claims.ExpiresAt.IsZero() {
		return Token{}, fmt.Errorf("JWT did not contain an exp claim")
	}
	if !m.now().Before(claims.ExpiresAt) {
		return Token{}, fmt.Errorf("JWT is expired")
	}

	user := strings.TrimSpace(address)
	if user == "" {
		user = claims.Address
	}
	if user == "" {
		return Token{}, fmt.Errorf("JWT did not contain a recognizable fmsg address; pass --address @user@example.com")
	}
	if !strings.HasPrefix(user, "@") {
		return Token{}, fmt.Errorf("fmsg address must start with @")
	}

	tok := Token{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   claims.ExpiresAt,
		User:        user,
	}
	creds := Credentials{
		Version:     credentialsVersion,
		AuthType:    authTypeJWT,
		AccessToken: tok.AccessToken,
		TokenType:   tok.TokenType,
		ExpiresAt:   tok.ExpiresAt,
		User:        tok.User,
		APIURL:      m.APIURL,
	}
	if err := Save(creds); err != nil {
		return Token{}, fmt.Errorf("saving credentials: %w", err)
	}
	return tok, nil
}

// tokenFromEnv exchanges an environment-supplied API key, reusing the
// in-memory token within a process and the on-disk token cache across
// processes (every CLI invocation is a new process — without the disk cache
// each one would exchange the key again). auth.json is never touched.
func (m *Manager) tokenFromEnv(ctx context.Context, apiKey string, forceRefresh bool) (Token, error) {
	if !forceRefresh && m.cachedTokenValid(m.envCache) {
		return tokenFromCredentials(m.envCache), nil
	}
	cachePath := tokenCachePath(m.APIURL, apiKey)
	if !forceRefresh {
		if creds, ok := loadCachedToken(cachePath); ok && m.cachedTokenValid(creds) {
			m.envCache = creds
			return tokenFromCredentials(creds), nil
		}
	}

	tok, err := m.exchange(ctx, apiKey)
	if err != nil {
		return Token{}, err
	}
	m.envCache = Credentials{
		Version:     credentialsVersion,
		AuthType:    authTypeAPIKey,
		AccessToken: tok.AccessToken,
		TokenType:   tok.TokenType,
		ExpiresAt:   tok.ExpiresAt,
		User:        tok.User,
		APIURL:      m.APIURL,
	}
	_ = saveCachedToken(cachePath, m.envCache) // best effort
	return tok, nil
}

func (m *Manager) cachedTokenValid(creds Credentials) bool {
	if creds.AccessToken == "" || creds.APIURL != m.APIURL {
		return false
	}
	return m.now().Add(refreshBefore).Before(creds.ExpiresAt)
}

func (m *Manager) tokenFromStoredJWT(creds Credentials, forceRefresh bool) (Token, error) {
	if creds.AccessToken == "" {
		return Token{}, fmt.Errorf("stored credentials are invalid; run fmsg login")
	}
	if creds.APIURL != "" && creds.APIURL != m.APIURL {
		return Token{}, fmt.Errorf("stored credentials are for %s; run fmsg login for %s", creds.APIURL, m.APIURL)
	}
	if forceRefresh {
		return Token{}, fmt.Errorf("stored user JWT cannot be refreshed; run fmsg login with a fresh token")
	}
	if !m.cachedTokenValid(creds) {
		return Token{}, fmt.Errorf("stored user JWT is expired or close to expiry; run fmsg login with a fresh token")
	}
	return tokenFromCredentials(creds), nil
}

func (m *Manager) exchange(ctx context.Context, apiKey string) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.APIURL+"/fmsg/token", nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := m.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("exchanging API key: network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Token{}, fmt.Errorf("API key was rejected; it may be invalid, expired, revoked, CIDR-blocked, or its sub-account may be unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return Token{}, fmt.Errorf("token exchange failed: API error %d: %s", resp.StatusCode, msg)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return Token{}, fmt.Errorf("decoding token response: %w", err)
	}
	return tr.token(m.now())
}

type tokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int64     `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (r tokenResponse) token(now time.Time) (Token, error) {
	if strings.TrimSpace(r.AccessToken) == "" {
		return Token{}, fmt.Errorf("token response did not include access_token")
	}
	tokenType := r.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	if !strings.EqualFold(tokenType, "Bearer") {
		return Token{}, fmt.Errorf("token response used unsupported token_type %q", r.TokenType)
	}

	expiresAt := r.ExpiresAt
	if expiresAt.IsZero() && r.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	if expiresAt.IsZero() {
		return Token{}, fmt.Errorf("token response did not include expires_at or expires_in")
	}

	user, err := jwtSubject(r.AccessToken)
	if err != nil {
		return Token{}, err
	}
	return Token{
		AccessToken: r.AccessToken,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        user,
	}, nil
}

func tokenFromCredentials(creds Credentials) Token {
	tokenType := creds.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return Token{
		AccessToken: creds.AccessToken,
		TokenType:   tokenType,
		ExpiresAt:   creds.ExpiresAt,
		User:        creds.User,
	}
}

func jwtSubject(token string) (string, error) {
	claims, err := jwtClaims(token)
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("access token did not contain a sub claim")
	}
	return claims.Subject, nil
}

type decodedClaims struct {
	Subject   string
	Address   string
	ExpiresAt time.Time
}

func jwtClaims(token string) (decodedClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return decodedClaims{}, fmt.Errorf("access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return decodedClaims{}, fmt.Errorf("decoding access token payload: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return decodedClaims{}, fmt.Errorf("decoding access token claims: %w", err)
	}

	claims := decodedClaims{
		Subject: stringClaim(raw, "sub"),
		Address: firstStringClaim(raw,
			"fmsg_address",
			"https://fmsg.dev/address",
			"address",
		),
	}
	if claims.Address == "" && strings.HasPrefix(claims.Subject, "@") {
		claims.Address = claims.Subject
	}
	if exp, ok := raw["exp"].(float64); ok && exp > 0 {
		claims.ExpiresAt = time.Unix(int64(exp), 0)
	}
	return claims, nil
}

func firstStringClaim(raw map[string]any, names ...string) string {
	for _, name := range names {
		if value := stringClaim(raw, name); value != "" {
			return value
		}
	}
	return ""
}

func stringClaim(raw map[string]any, name string) string {
	value, _ := raw[name].(string)
	return strings.TrimSpace(value)
}
