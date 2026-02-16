package chat

import (
	"context"
	"strings"
	"sync"

	"github.com/gempir/go-twitch-irc/v4"

	"github.com/Guliveer/twitch-miner-go/internal/logger"
)

// Manager manages IRC chat connections for multiple streamers.
// It uses the go-twitch-irc library which handles PING/PONG keepalive
// and automatic reconnection internally.
type Manager struct {
	mu sync.Mutex

	client *twitch.Client
	handler *Handler

	username string
	authToken string

	channels map[string]bool
	running bool

	log *logger.Logger
}

// NewManager creates a new IRC chat Manager.
func NewManager(username, authToken string, log *logger.Logger) *Manager {
	handler := NewHandler(username, log)

	client := twitch.NewClient(username, "oauth:"+authToken)

	manager := &Manager{
		client:    client,
		handler:   handler,
		username:  username,
		authToken: authToken,
		channels:  make(map[string]bool),
		log:       log,
	}

	client.OnPrivateMessage(handler.OnPrivateMessage)
	client.OnConnect(handler.OnConnect)
	client.OnReconnectMessage(func(msg twitch.ReconnectMessage) {
		handler.OnReconnect()
	})
	client.OnSelfJoinMessage(handler.OnSelfJoinMessage)
	client.OnSelfPartMessage(handler.OnSelfPartMessage)

	return manager
}

// Join joins a channel for chat presence. The channel name should be the
// streamer's username (without the # prefix).
func (m *Manager) Join(channelName string) error {
	channel := strings.ToLower(channelName)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.channels[channel] {
		m.log.Debug("Already in IRC", "channel", channel)
		return nil
	}

	m.channels[channel] = true
	m.client.Join(channel)
	m.log.Info("Join IRC Chat", "channel", channel)

	return nil
}

// Leave leaves a channel.
func (m *Manager) Leave(channelName string) error {
	channel := strings.ToLower(channelName)

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.channels[channel] {
		m.log.Debug("Not in IRC", "channel", channel)
		return nil
	}

	delete(m.channels, channel)
	m.client.Depart(channel)
	m.log.Info("Leave IRC Chat", "channel", channel)

	return nil
}

// Run connects to Twitch IRC and maintains presence. It blocks until the
// context is cancelled. The go-twitch-irc library handles reconnection
// automatically.
func (m *Manager) Run(ctx context.Context) error {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		err := m.client.Connect()
		if err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		m.Close()
		return ctx.Err()
	case err := <-errCh:
		if err != nil && ctx.Err() == nil {
			m.log.Error("IRC connection error", "error", err)
			return err
		}
		return ctx.Err()
	}
}

// Close disconnects from all channels and shuts down the IRC client.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}
	m.running = false

	for channel := range m.channels {
		m.client.Depart(channel)
		m.log.Info("Leave IRC Chat", "channel", channel)
	}
	m.channels = make(map[string]bool)

	if err := m.client.Disconnect(); err != nil {
		m.log.Debug("IRC disconnect", "error", err)
	}

	m.log.Info("IRC chat manager closed")
}

// IsJoined returns whether the manager is currently in the given channel.
func (m *Manager) IsJoined(channelName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.channels[strings.ToLower(channelName)]
}

// JoinedChannels returns a list of currently joined channels.
func (m *Manager) JoinedChannels() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	channels := make([]string, 0, len(m.channels))
	for channelName := range m.channels {
		channels = append(channels, channelName)
	}
	return channels
}
