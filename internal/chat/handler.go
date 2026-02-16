package chat

import (
	"context"
	"strings"

	"github.com/gempir/go-twitch-irc/v4"

	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Handler processes incoming IRC chat messages, detecting @mentions
// and logging connection events.
type Handler struct {
	username string
	log *logger.Logger
}

// NewHandler creates a new chat message Handler.
func NewHandler(username string, log *logger.Logger) *Handler {
	return &Handler{
		username: strings.ToLower(username),
		log:      log,
	}
}

// OnPrivateMessage is called when a chat message is received.
// It checks for @mentions of the bot username and logs them.
func (h *Handler) OnPrivateMessage(msg twitch.PrivateMessage) {
	msgLower := strings.ToLower(msg.Message)
	mention := "@" + h.username

	if strings.Contains(msgLower, mention) || strings.Contains(msgLower, h.username) {
		h.log.Event(
			context.Background(),
			model.EventChatMention,
			"Chat mention detected",
			"nick", msg.User.DisplayName,
			"channel", msg.Channel,
			"message", msg.Message,
		)
	}
}

// OnConnect is called when the IRC client connects to the server.
func (h *Handler) OnConnect() {
	h.log.Info("💬 Connected to Twitch IRC")
}

// OnReconnect is called when the IRC client reconnects to the server.
func (h *Handler) OnReconnect() {
	h.log.Info("💬 Reconnected to Twitch IRC")
}

// OnSelfJoinMessage is called when the bot joins a channel.
func (h *Handler) OnSelfJoinMessage(msg twitch.UserJoinMessage) {
	h.log.Info("💬 Joined IRC chat", "channel", msg.Channel)
}

// OnSelfPartMessage is called when the bot leaves a channel.
func (h *Handler) OnSelfPartMessage(msg twitch.UserPartMessage) {
	h.log.Info("💬 Left IRC chat", "channel", msg.Channel)
}
