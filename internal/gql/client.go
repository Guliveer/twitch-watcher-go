// Package gql provides a typed GraphQL client for the Twitch GQL API.
// It handles connection pooling, request building, client version caching,
// rate limiting awareness, and error handling with retries.
package gql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
)

// ErrCircuitOpen is returned when the circuit breaker is open and requests
// are being skipped to avoid hammering a failing API.
var ErrCircuitOpen = errors.New("circuit breaker open: API requests temporarily suspended")

// integrityFailureOps lists GQL operations where integrity check failures are
// expected and should be logged at DEBUG instead of WARN. These operations
// sometimes fail with "failed integrity check" but may still succeed on retry
var integrityFailureOps = map[string]bool{
	"JoinRaid":              true,
	"ClaimCommunityPoints":  true,
	"ViewerDropsDashboard":  true,
}

// circuitBreaker tracks consecutive failures and backs off when the API
type circuitBreaker struct {
	mu               sync.Mutex
	consecutiveFails int
	lastFailure      time.Time
	cooldownUntil    time.Time
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	cb.consecutiveFails = 0
	cb.mu.Unlock()
}

// recordFailure increments the failure counter and, after 10 consecutive
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	cb.consecutiveFails++
	cb.lastFailure = time.Now()
	if cb.consecutiveFails >= 10 {
		backoff := time.Duration(cb.consecutiveFails-9) * 30 * time.Second
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
		cb.cooldownUntil = time.Now().Add(backoff)
	}
	cb.mu.Unlock()
}

// shouldSkip returns true if the circuit breaker is open and requests
func (cb *circuitBreaker) shouldSkip() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return time.Now().Before(cb.cooldownUntil)
}

// Client is the Twitch GQL HTTP client with connection pooling,
// client version caching, circuit breaker, and retry logic.
type Client struct {
	httpClient   *http.Client
	transport    *http.Transport
	auth         auth.Provider
	log          *logger.Logger
	versionCache *versionCache
	breaker      *circuitBreaker

	maxRetries int
	mu         sync.RWMutex
}

// NewClient creates a new GQL Client with a shared HTTP client configured
// for connection pooling and the given authenticator.
func NewClient(authenticator auth.Provider, log *logger.Logger) *Client {
	transport := &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   constants.DefaultHTTPTimeout,
	}

	return &Client{
		httpClient:   httpClient,
		transport:    transport,
		auth:         authenticator,
		log:          log,
		versionCache: newVersionCache(),
		breaker:      &circuitBreaker{},
		maxRetries:   constants.DefaultMaxRetries,
	}
}

// SetStartupMode configures the client for fast startup with reduced
// timeout and retries. Call SetNormalMode to restore defaults.
func (c *Client) SetStartupMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.httpClient.Timeout = constants.StartupHTTPTimeout
	c.maxRetries = constants.StartupMaxRetries
	c.log.Debug("GQL client switched to startup mode",
		"timeout", constants.StartupHTTPTimeout,
		"max_retries", constants.StartupMaxRetries)
}

// SetNormalMode restores the client to normal operating mode with
// default timeout and retries.
func (c *Client) SetNormalMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.httpClient.Timeout = constants.DefaultHTTPTimeout
	c.maxRetries = constants.DefaultMaxRetries
	c.log.Debug("GQL client switched to normal mode",
		"timeout", constants.DefaultHTTPTimeout,
		"max_retries", constants.DefaultMaxRetries)
}

func (c *Client) getMaxRetries() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxRetries
}

// HTTPClient returns the underlying *http.Client for reuse by other packages
// (e.g., minute-watched events that need the same connection pool).
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

type gqlRequest struct {
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables,omitempty"`
	Extensions    *gqlExtensions `json:"extensions,omitempty"`
	Query         string         `json:"query,omitempty"`
}

type gqlExtensions struct {
	PersistedQuery *persistedQuery `json:"persistedQuery"`
}

type persistedQuery struct {
	Version    int    `json:"version"`
	SHA256Hash string `json:"sha256Hash"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
	Path    []any  `json:"path,omitempty"`
}

// PostGQL sends a single GQL operation and returns the "data" portion of the response.
// It builds the request body, adds auth and version headers, handles errors, and
// retries on transient failures (429, 5xx) with exponential backoff.
func (c *Client) PostGQL(ctx context.Context, op constants.GQLOperation, variables map[string]any) (json.RawMessage, error) {
	reqBody := c.buildRequestBody(op, variables)
	return c.doGQLRequest(ctx, reqBody, op.OperationName)
}

// PostGQLBatch sends multiple GQL operations in a single HTTP request (batch).
// Twitch supports batched GQL requests as a JSON array.
func (c *Client) PostGQLBatch(ctx context.Context, ops []constants.GQLOperation, varsList []map[string]any) ([]json.RawMessage, error) {
	if len(ops) != len(varsList) {
		return nil, fmt.Errorf("ops and varsList must have the same length")
	}

	batch := make([]gqlRequest, len(ops))
	for i, op := range ops {
		batch[i] = c.buildRequestBody(op, varsList[i])
	}

	jsonBody, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshaling batch GQL request: %w", err)
	}

	respBody, err := c.doHTTPRequest(ctx, jsonBody, "batch")
	if err != nil {
		return nil, err
	}

	var responses []gqlResponse
	if err := json.Unmarshal(respBody, &responses); err != nil {
		return nil, fmt.Errorf("parsing batch GQL response: %w", err)
	}

	results := make([]json.RawMessage, len(responses))
	for i, r := range responses {
		if len(r.Errors) > 0 {
			c.log.Warn("GQL batch error",
				"index", i,
				"error", r.Errors[0].Message)
		}
		results[i] = r.Data
	}

	return results, nil
}

func (c *Client) buildRequestBody(op constants.GQLOperation, variables map[string]any) gqlRequest {
	req := gqlRequest{
		OperationName: op.OperationName,
		Variables:     variables,
	}

	if op.Query != "" {
		req.Query = op.Query
	} else {
		req.Extensions = &gqlExtensions{
			PersistedQuery: &persistedQuery{
				Version:    1,
				SHA256Hash: op.SHA256Hash,
			},
		}
	}

	return req
}

func (c *Client) doGQLRequest(ctx context.Context, reqBody gqlRequest, opName string) (json.RawMessage, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling GQL request: %w", err)
	}

	respBody, err := c.doHTTPRequest(ctx, jsonBody, opName)
	if err != nil {
		return nil, err
	}

	var response gqlResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("parsing GQL response for %s: %w", opName, err)
	}

	if len(response.Errors) > 0 {
		errMsg := response.Errors[0].Message
		if strings.Contains(errMsg, "integrity check") && integrityFailureOps[opName] {
			c.log.Debug("GQL integrity check failure (expected)",
				"operation", opName,
				"error", errMsg)
		} else {
			c.log.Warn("GQL operation returned errors",
				"operation", opName,
				"error", errMsg)
		}
	}

	return response.Data, nil
}

// doHTTPRequest performs the actual HTTP POST with auth headers, client version,
// integrity token, and retry logic for transient errors. The number of retries
// is controlled by the client's maxRetries setting (configurable via
// SetStartupMode/SetNormalMode).
//
// Retry logging strategy: individual retries are logged at DEBUG level to
// reduce noise. Only the final failure (after all retries exhausted) is
// logged at WARN level. Known-flaky operations (e.g., VideoPlayerStreamInfoOverlayChannel)
func (c *Client) doHTTPRequest(ctx context.Context, jsonBody []byte, opName string) ([]byte, error) {
	if c.breaker.shouldSkip() {
		c.log.Debug("Circuit breaker open, skipping request", "operation", opName)
		return nil, ErrCircuitOpen
	}

	maxRetries := c.getMaxRetries()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			c.log.Debug("Retrying GQL request",
				"operation", opName,
				"attempt", fmt.Sprintf("%d/%d", attempt, maxRetries),
				"backoff", backoff)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, constants.GQLURL,
			bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("creating GQL request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		for k, v := range c.auth.GetAuthHeaders() {
			req.Header.Set(k, v)
		}
		req.Header.Set("Client-Version", c.updateClientVersion(ctx))

		if integrityToken, err := c.auth.FetchIntegrityToken(ctx); err != nil {
			c.log.Debug("Failed to fetch integrity token, proceeding without it",
				"operation", opName, "error", err)
		} else if integrityToken != "" {
			req.Header.Set("Client-Integrity", integrityToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries {
				c.log.Debug("GQL request failed, will retry",
					"operation", opName,
					"attempt", fmt.Sprintf("%d/%d", attempt+1, maxRetries),
					"error", err)
				continue
			}
			c.log.Warn("GQL request failed after all retries",
				"operation", opName,
				"attempts", maxRetries+1,
				"error", err)
			return nil, fmt.Errorf("GQL request for %s failed: %w", opName, err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if readErr != nil {
			if attempt < maxRetries {
				c.log.Debug("Failed to read GQL response, will retry",
					"operation", opName,
					"attempt", fmt.Sprintf("%d/%d", attempt+1, maxRetries),
					"error", readErr)
				continue
			}
			return nil, fmt.Errorf("reading GQL response for %s: %w", opName, readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < maxRetries {
				c.log.Debug("GQL request returned retryable status, will retry",
					"operation", opName,
					"status", resp.StatusCode,
					"attempt", fmt.Sprintf("%d/%d", attempt+1, maxRetries))
				continue
			}
			c.log.Warn("GQL request returned retryable status after all retries",
				"operation", opName,
				"status", resp.StatusCode,
				"attempts", maxRetries+1)
			return nil, fmt.Errorf("GQL request for %s returned status %d after %d retries",
				opName, resp.StatusCode, maxRetries)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GQL request for %s returned status %d: %s",
				opName, resp.StatusCode, string(body))
		}

		c.breaker.recordSuccess()
		c.log.Debug("GQL request completed",
			"operation", opName,
			"status", resp.StatusCode)

		return body, nil
	}

	c.breaker.recordFailure()
	return nil, fmt.Errorf("GQL request for %s exhausted retries", opName)
}
