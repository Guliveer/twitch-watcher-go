package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"golang.org/x/term"
)

type loginResponse struct {
	AccessToken string `json:"access_token"`
	ErrorCode   int    `json:"error_code"`
	ErrorMsg    string `json:"error"`
}

var twitchLoginErrorCodes = map[int]string{
	1000:  "captcha required (try again or use auth token)",
	3001:  "invalid login credentials",
	3003:  "invalid login credentials",
	3011:  "two-factor authentication required (Authy)",
	3012:  "invalid two-factor code",
	3022:  "two-factor authentication required (email/SMS)",
	3023:  "two-factor authentication required (email/SMS)",
	5023:  "too many login attempts — wait and try again",
	5027:  "integrity check failed",
	10001: "account locked — contact Twitch support",
}

// loginWithPassword performs the Twitch passport login flow using username/password.
// It handles 2FA challenges (Authy and TwitchGuard/email/SMS).
func (a *Authenticator) loginWithPassword(ctx context.Context, password string) error {
	a.log.Info("Attempting password-based login", "username", a.username)

	payload := map[string]any{
		"username":      a.username,
		"password":      password,
		"client_id":     constants.ClientIDBrowser,
		"undelete_user": false,
		"remember_me":   true,
	}

	resp, err := a.sendLoginRequest(ctx, payload)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}

	if resp.ErrorCode == 3011 || resp.ErrorCode == 3012 {
		return a.handle2FA(ctx, password, payload, "authy_token", "Authy 2FA")
	}
	if resp.ErrorCode == 3022 || resp.ErrorCode == 3023 {
		return a.handle2FA(ctx, password, payload, "twitchguard_code", "email/SMS 2FA")
	}

	if resp.AccessToken != "" {
		return a.finalizeLogin(ctx, resp.AccessToken)
	}

	if desc, ok := twitchLoginErrorCodes[resp.ErrorCode]; ok {
		return fmt.Errorf("twitch login failed (code %d): %s", resp.ErrorCode, desc)
	}
	if resp.ErrorMsg != "" {
		return fmt.Errorf("twitch login failed: %s (code %d)", resp.ErrorMsg, resp.ErrorCode)
	}
	return fmt.Errorf("twitch login failed with unknown error code %d", resp.ErrorCode)
}

func (a *Authenticator) handle2FA(ctx context.Context, password string, basePayload map[string]any, tokenKey, label string) error {
	a.log.Info("Two-factor authentication required", "type", label)

	code, err := promptLine(fmt.Sprintf("Enter your %s code: ", label))
	if err != nil {
		return fmt.Errorf("reading 2FA code: %w", err)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("2FA code cannot be empty")
	}

	payload := map[string]any{
		"username":      a.username,
		"password":      password,
		"client_id":     constants.ClientIDBrowser,
		"undelete_user": false,
		"remember_me":   true,
		tokenKey:        code,
	}

	resp, err := a.sendLoginRequest(ctx, payload)
	if err != nil {
		return fmt.Errorf("2FA login request failed: %w", err)
	}

	if resp.AccessToken != "" {
		return a.finalizeLogin(ctx, resp.AccessToken)
	}

	if desc, ok := twitchLoginErrorCodes[resp.ErrorCode]; ok {
		return fmt.Errorf("2FA login failed (code %d): %s", resp.ErrorCode, desc)
	}
	if resp.ErrorMsg != "" {
		return fmt.Errorf("2FA login failed: %s (code %d)", resp.ErrorMsg, resp.ErrorCode)
	}
	return fmt.Errorf("2FA login failed with unknown error code %d", resp.ErrorCode)
}

func (a *Authenticator) sendLoginRequest(ctx context.Context, payload map[string]any) (*loginResponse, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling login payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.LoginURL,
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("creating login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Client-Id", constants.ClientIDBrowser)
	req.Header.Set("X-Device-Id", a.deviceID)
	req.Header.Set("User-Agent", constants.DefaultUserAgent)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing login response: %w", err)
	}

	return &result, nil
}

func (a *Authenticator) finalizeLogin(ctx context.Context, token string) error {
	a.authToken = token

	if err := a.validateToken(ctx); err != nil {
		return fmt.Errorf("login succeeded but token validation failed: %w", err)
	}

	a.log.Info("Successfully authenticated via password login",
		"username", a.username, "user_id", a.userID)

	a.saveCookies()
	return nil
}

func (a *Authenticator) saveCookies() {
	a.cookieJar.Set("auth-token", a.authToken)
	if a.userID != "" {
		a.cookieJar.Set("persistent", a.userID)
	}
	if err := a.cookieJar.Save(a.cookieFile); err != nil {
		a.log.Warn("Failed to save cookies", "error", err)
	} else {
		a.log.Info("Cookies saved", "file", a.cookieFile)
	}
}

// isInteractiveTerminal returns true if stdin is connected to a terminal
func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptPassword reads a password from stdin without echoing it.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return string(password), nil
}

func promptLine(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
