// Package server provides a lightweight HTTP analytics server that exposes
// streamer data, statistics, and a simple dashboard.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// StreamerFunc is a function that returns the current list of streamers
// across all miners. Used to dynamically fetch streamer data.
type StreamerFunc func() []*model.Streamer

// AccountStatusFunc is a function that returns the running status of each
// miner account. Used to dynamically fetch account health data.
type AccountStatusFunc func() []AccountStatus

// AccountStatus represents the running state of a single miner account.
type AccountStatus struct {
	Username string `json:"username"`
	Running  bool   `json:"running"`
}

// AnalyticsServer serves the analytics dashboard and JSON API endpoints.
type AnalyticsServer struct {
	addr string
	log  *logger.Logger
	srv  *http.Server

	mu                sync.RWMutex
	streamers         []*model.Streamer
	streamerFunc      StreamerFunc
	accountStatusFunc AccountStatusFunc
}

// NewAnalyticsServer creates a new AnalyticsServer bound to the given address.
func NewAnalyticsServer(addr string, log *logger.Logger) *AnalyticsServer {
	s := &AnalyticsServer{
		addr: addr,
		log:  log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/streamers", s.handleStreamers)
	mux.HandleFunc("GET /api/streamer/{name}", s.handleStreamer)
	mux.HandleFunc("GET /api/stats", s.handleStats)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           withLogging(log, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	return s
}

// SetStreamers updates the streamer list reference. Thread-safe.
func (s *AnalyticsServer) SetStreamers(streamers []*model.Streamer) {
	s.mu.Lock()
	s.streamers = streamers
	s.mu.Unlock()
}

// SetStreamerFunc sets a function that dynamically returns all streamers
// across all miners. When set, getStreamers() calls this function instead
// of returning the static list.
func (s *AnalyticsServer) SetStreamerFunc(fn StreamerFunc) {
	s.mu.Lock()
	s.streamerFunc = fn
	s.mu.Unlock()
}

// SetAccountStatusFunc sets a function that dynamically returns the running
// status of all miner accounts. Thread-safe.
func (s *AnalyticsServer) SetAccountStatusFunc(fn AccountStatusFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountStatusFunc = fn
}

// getAccountStatuses returns the current account statuses. Thread-safe.
func (s *AnalyticsServer) getAccountStatuses() []AccountStatus {
	s.mu.RLock()
	fn := s.accountStatusFunc
	s.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return nil
}

// getStreamers returns the current streamer list. Thread-safe.
func (s *AnalyticsServer) getStreamers() []*model.Streamer {
	s.mu.RLock()
	fn := s.streamerFunc
	s.mu.RUnlock()

	if fn != nil {
		return fn()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streamers
}

// Run starts the HTTP server and blocks until the context is cancelled.
// It performs graceful shutdown when the context is done.
func (s *AnalyticsServer) Run(ctx context.Context) error {
	s.log.Info("Analytics server starting", "addr", s.addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("analytics server: %w", err)
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		s.log.Info("Analytics server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), constants.DefaultGracefulShutdownTimeout)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("analytics server shutdown: %w", err)
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func withLogging(log *logger.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration", time.Since(start).String(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before writing it.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
