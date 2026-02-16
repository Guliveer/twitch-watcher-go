package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
)

// DeviceCodeResponse represents the response from the device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
}

// TokenResponse represents a successful token response.
type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	ExpiresIn    int      `json:"expires_in"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

// TokenErrorResponse represents an error response from the token endpoint.
type TokenErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// loginWithDeviceCode orchestrates the full Twitch device code login flow.
// It requests a device code, displays the verification URI and user code,
func (a *Authenticator) loginWithDeviceCode(ctx context.Context) error {
	dcResp, err := a.requestDeviceCode(ctx)
	if err != nil {
		return fmt.Errorf("requesting device code: %w", err)
	}

	fmt.Println()
	fmt.Printf("📺 Device Code Login [%s]\n", a.username)
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("Go to: %s\n", dcResp.VerificationURI)
	fmt.Printf("Enter code: %s\n", dcResp.UserCode)
	fmt.Println("─────────────────────────────────────")
	fmt.Println("Waiting for authorization...")
	fmt.Println()

	tokenResp, err := a.pollForToken(ctx, dcResp.DeviceCode, dcResp.Interval, dcResp.ExpiresIn)
	if err != nil {
		return fmt.Errorf("polling for token: %w", err)
	}

	a.authToken = tokenResp.AccessToken

	if err := a.validateToken(ctx); err != nil {
		return fmt.Errorf("device code login succeeded but token validation failed: %w", err)
	}

	a.cookieJar.Set("auth-token", a.authToken)
	if tokenResp.RefreshToken != "" {
		a.cookieJar.Set("refresh-token", tokenResp.RefreshToken)
	}
	if a.userID != "" {
		a.cookieJar.Set("persistent", a.userID)
	}
	if err := a.cookieJar.Save(a.cookieFile); err != nil {
		a.log.Warn("Failed to save cookies", "error", err)
	} else {
		a.log.Info("Cookies saved", "file", a.cookieFile)
	}

	a.log.Info("Successfully authenticated via device code flow",
		"username", a.username, "user_id", a.userID)

	return nil
}

// requestDeviceCode sends a POST to the Twitch device code endpoint and
func (a *Authenticator) requestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	form := url.Values{
		"client_id": {constants.ClientID},
		"scopes":    {constants.DeviceCodeScopes},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.DeviceCodeURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating device code request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending device code request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var dcResp DeviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("parsing device code response: %w", err)
	}

	if dcResp.DeviceCode == "" || dcResp.UserCode == "" {
		return nil, fmt.Errorf("device code response missing required fields")
	}

	return &dcResp, nil
}

// pollForToken polls the Twitch token endpoint until the user authorizes the
func (a *Authenticator) pollForToken(ctx context.Context, deviceCode string, interval, expiresIn int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}

	pollInterval := time.Duration(interval) * time.Second
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("device code login cancelled: %w", ctx.Err())
		case t := <-ticker.C:
			if t.After(deadline) {
				return nil, fmt.Errorf("device code expired, please try again")
			}

			tokenResp, err := a.requestToken(ctx, deviceCode)
			if err != nil {
				return nil, err
			}

			if tokenResp != nil {
				return tokenResp, nil
			}

		}
	}
}

// requestToken makes a single token request to the Twitch token endpoint.
// Returns (*TokenResponse, nil) on success, (nil, nil) if authorization is
func (a *Authenticator) requestToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	form := url.Values{
		"client_id":   {constants.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
		if tokenResp.AccessToken == "" {
			return nil, fmt.Errorf("token response missing access_token")
		}
		return &tokenResp, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errResp TokenErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("parsing token error response: %w", err)
		}

		switch errResp.Message {
		case "authorization_pending":
			return nil, nil
		case "slow_down":
			a.log.Debug("Token endpoint requested slow down")
			return nil, nil
		case "expired_token":
			return nil, fmt.Errorf("device code expired, please try again")
		default:
			return nil, fmt.Errorf("token request failed: %s (status %d)", errResp.Message, errResp.Status)
		}
	}

	return nil, fmt.Errorf("token request returned unexpected HTTP %d: %s",
		resp.StatusCode, strings.TrimSpace(string(body)))
}

// refreshAccessToken attempts to refresh the OAuth token using a stored refresh token.
// It loads the refresh token from cookies, exchanges it for a new access token,
func (a *Authenticator) refreshAccessToken(ctx context.Context) error {
	refreshToken := a.cookieJar.Get("refresh-token")
	if refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	a.log.Info("Attempting token refresh")

	form := url.Values{
		"client_id":     {constants.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh request returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parsing refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("refresh response missing access_token")
	}

	a.authToken = tokenResp.AccessToken

	if err := a.validateToken(ctx); err != nil {
		return fmt.Errorf("refreshed token validation failed: %w", err)
	}

	a.cookieJar.Set("auth-token", a.authToken)
	if tokenResp.RefreshToken != "" {
		a.cookieJar.Set("refresh-token", tokenResp.RefreshToken)
	}
	if a.userID != "" {
		a.cookieJar.Set("persistent", a.userID)
	}
	if err := a.cookieJar.Save(a.cookieFile); err != nil {
		a.log.Warn("Failed to save cookies after refresh", "error", err)
	} else {
		a.log.Info("Cookies saved after token refresh", "file", a.cookieFile)
	}

	a.log.Info("Successfully refreshed access token",
		"username", a.username, "user_id", a.userID)

	return nil
}
