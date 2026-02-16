package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
)

// Authenticator handles Twitch login, token management, and cookie persistence.
// It is safe for concurrent use.
type Authenticator struct {
	mu sync.RWMutex

	username      string
	authToken     string
	userID        string
	deviceID      string
	clientSession string

	cookieJar  *CookieJar
	cookieFile string

	cfg        config.AuthConfig
	log        *logger.Logger
	httpClient *http.Client

	integrityToken  string
	integrityExpire int64
}

// NewAuthenticator creates a new Authenticator from the account configuration.
// The cookie file path is automatically derived as cookies/{username}.json.
// On Fly.io / Docker, DATA_DIR points to the persistent volume (e.g. /data),
// so cookies are stored under {DATA_DIR}/cookies/{username}.json instead.
func NewAuthenticator(cfg *config.AccountConfig, log *logger.Logger) *Authenticator {
	username := strings.ToLower(cfg.Username)
	cookiesDir := "cookies"
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		cookiesDir = filepath.Join(dataDir, "cookies")
	}
	cookieFile := filepath.Join(cookiesDir, username+".json")

	if err := os.MkdirAll(cookiesDir, 0o755); err != nil {
		log.Warn("Failed to create cookies directory", "dir", cookiesDir, "error", err)
	}

	return &Authenticator{
		username:      cfg.Username,
		cfg:           cfg.Auth,
		cookieFile:    cookieFile,
		cookieJar:     NewCookieJar(),
		deviceID:      generateDeviceID(),
		clientSession: GenerateHex(16),
		log:           log,
		httpClient: &http.Client{
			Timeout: constants.DefaultHTTPTimeout,
		},
	}
}

// Login performs the authentication flow with the following priority:
//  1. Load cookies from file → validate token → success
//     1b. If token expired, try refresh token → validate → save → success
//  2. Auth token from config/env var → validate → save cookies → success
//  3. Password from config/env var (TWITCH_PASSWORD_<USERNAME>) → login → save → success
//  4. Device Code Flow (TV-style login) → display code → poll → save → success
func (a *Authenticator) Login(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if CookieFileExists(a.cookieFile) {
		if os.Getenv("DATA_DIR") != "" {
			a.log.Info("Loading cookies from persistent volume", "file", a.cookieFile)
		} else {
			a.log.Info("Loading existing cookies", "file", a.cookieFile)
		}
		if err := a.cookieJar.Load(a.cookieFile); err != nil {
			a.log.Warn("Failed to load cookies, will try other methods", "error", err)
		} else {
			token := a.cookieJar.Get("auth-token")
			if token != "" {
				a.authToken = token
				if err := a.validateToken(ctx); err == nil {
					a.log.Info("Successfully authenticated from cookies",
						"username", a.username, "user_id", a.userID)
					return nil
				}
				a.log.Warn("Cached token is invalid, will try refresh")
				a.authToken = ""

				if err := a.refreshAccessToken(ctx); err == nil {
					a.log.Info("Successfully authenticated via token refresh",
						"username", a.username, "user_id", a.userID)
					return nil
				}
				a.log.Warn("Token refresh failed, will try other methods")
			}
		}
	}

	if a.cfg.AuthToken != "" {
		a.log.Info("Using auth token from config/environment")
		a.authToken = a.cfg.AuthToken
		if err := a.validateToken(ctx); err != nil {
			a.log.Warn("Auth token from config is invalid, will try other methods", "error", err)
			a.authToken = ""
		} else {
			a.log.Info("Successfully authenticated with config auth token",
				"username", a.username, "user_id", a.userID)
			a.saveCookies()
			return nil
		}
	}

	envKey := "TWITCH_AUTH_TOKEN_" + strings.ToUpper(strings.ReplaceAll(a.username, "-", "_"))
	if envToken := os.Getenv(envKey); envToken != "" && envToken != a.cfg.AuthToken {
		a.log.Info("Using auth token from environment variable", "env", envKey)
		a.authToken = envToken
		if err := a.validateToken(ctx); err != nil {
			a.log.Warn("Auth token from env is invalid, will try other methods", "error", err)
			a.authToken = ""
		} else {
			a.log.Info("Successfully authenticated with env auth token",
				"username", a.username, "user_id", a.userID)
			a.saveCookies()
			return nil
		}
	}

	password := a.cfg.Password
	if password == "" {
		pwEnvKey := "TWITCH_PASSWORD_" + strings.ToUpper(strings.ReplaceAll(a.username, "-", "_"))
		password = os.Getenv(pwEnvKey)
	}
	if password != "" {
		a.log.Info("Attempting login with password from config/environment")
		if err := a.loginWithPassword(ctx, password); err != nil {
			a.log.Warn("Password login failed", "error", err)
		} else {
			return nil
		}
	}

	a.log.Error("No valid credentials found — starting device code login", "account", a.username)
	if err := a.loginWithDeviceCode(ctx); err != nil {
		return fmt.Errorf("device code login failed: %w", err)
	}
	return nil
}

// twitchValidateURL is the Twitch OAuth2 token validation endpoint.
const twitchValidateURL = "https://id.twitch.tv/oauth2/validate"

// validateToken checks if the current auth token is valid and belongs to the
// expected user by calling the Twitch OAuth2 validate endpoint. This replaces
// the previous GQL-based approach because the OAuth2 endpoint both validates
func (a *Authenticator) validateToken(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, twitchValidateURL, nil)
	if err != nil {
		return fmt.Errorf("create validate request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+a.authToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token validation failed with status %d", resp.StatusCode)
	}

	var result struct {
		Login  string `json:"login"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode validate response: %w", err)
	}

	if !strings.EqualFold(result.Login, a.username) {
		return fmt.Errorf("authenticated as %q but config expects %q — please log in with the correct account",
			result.Login, a.username)
	}

	a.userID = result.UserID
	return nil
}

// AuthToken returns the current OAuth token.
func (a *Authenticator) AuthToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.authToken
}

// UserID returns the authenticated user's Twitch numeric ID.
func (a *Authenticator) UserID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.userID
}

// DeviceID returns the device ID used for API requests.
func (a *Authenticator) DeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.deviceID
}

// ClientSession returns the client session ID.
func (a *Authenticator) ClientSession() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.clientSession
}

// Username returns the Twitch login name.
func (a *Authenticator) Username() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.username
}

// GetAuthHeaders returns the headers needed for all Twitch API requests.
func (a *Authenticator) GetAuthHeaders() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]string{
		"Authorization":     "OAuth " + a.authToken,
		"Client-Id":         constants.ClientID,
		"Client-Session-Id": a.clientSession,
		"X-Device-Id":       a.deviceID,
		"User-Agent":        constants.DefaultUserAgent,
	}
}

// FetchIntegrityToken fetches or returns a cached integrity token from
// https://gql.twitch.tv/integrity. The token is refreshed 5 minutes before expiry.
func (a *Authenticator) FetchIntegrityToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	nowMs := time.Now().UnixMilli()
	if a.integrityToken != "" && (a.integrityExpire-nowMs) > 5*60*1000 {
		return a.integrityToken, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.IntegrityURL,
		strings.NewReader("{}"))
	if err != nil {
		return a.integrityToken, fmt.Errorf("creating integrity request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "OAuth "+a.authToken)
	req.Header.Set("Client-Id", constants.ClientID)
	req.Header.Set("Client-Session-Id", a.clientSession)
	req.Header.Set("X-Device-Id", a.deviceID)
	req.Header.Set("User-Agent", constants.DefaultUserAgent)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return a.integrityToken, fmt.Errorf("fetching integrity token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return a.integrityToken, fmt.Errorf("reading integrity response: %w", err)
	}

	var result struct {
		Token      string `json:"token"`
		Expiration int64  `json:"expiration"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return a.integrityToken, fmt.Errorf("parsing integrity response: %w", err)
	}

	a.integrityToken = result.Token
	a.integrityExpire = result.Expiration

	a.log.Debug("Refreshed integrity token", "expires_in_ms", result.Expiration-nowMs)
	return a.integrityToken, nil
}

// generateDeviceID creates a random 32-character alphanumeric device ID,
func generateDeviceID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
			for i := range randomBytes {
			randomBytes[i] = charset[i%len(charset)]
		}
		return string(randomBytes)
	}
	for i := range randomBytes {
		randomBytes[i] = charset[int(randomBytes[i])%len(charset)]
	}
	return string(randomBytes)
}

// GenerateHex creates a random hex string of the given byte length.
// It is exported for use by other packages that need transaction IDs.
func GenerateHex(numBytes int) string {
	randomBytes := make([]byte, numBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return strings.Repeat("0", numBytes*2)
	}
	return fmt.Sprintf("%x", randomBytes)
}
