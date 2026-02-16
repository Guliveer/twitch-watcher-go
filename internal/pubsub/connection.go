package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Connection represents a single WebSocket connection to the Twitch PubSub server.
// Each connection can subscribe to up to MaxTopicsPerConn (50) topics.
type Connection struct {
	mu sync.Mutex

	index int
	conn *websocket.Conn
	topics []*model.PubSubTopic
	pendingTopics []*model.PubSubTopic

	lastPong time.Time
	isConnected bool

	messages chan *model.Message
	writeCh chan []byte

	auth auth.Provider
	log *logger.Logger

	nonceToTopic map[string]string

	lastMsgTimestamp time.Time
	lastMsgIdentifier string
}

// NewConnection creates a new PubSub Connection and dials the Twitch PubSub server.
func NewConnection(ctx context.Context, index int, authProvider auth.Provider, log *logger.Logger) (*Connection, error) {
	conn, _, err := websocket.Dial(ctx, constants.PubSubURL, &websocket.DialOptions{})
	if err != nil {
		return nil, fmt.Errorf("dialing PubSub server: %w", err)
	}

	conn.SetReadLimit(128 << 10) // 128 KB

	connection := &Connection{
		index:        index,
		conn:         conn,
		topics:       make([]*model.PubSubTopic, 0, constants.MaxTopicsPerConn),
		messages:     make(chan *model.Message, 32),
		writeCh:      make(chan []byte, 64),
		auth:         authProvider,
		log:          log,
		nonceToTopic: make(map[string]string),
		lastPong:     time.Now(),
		isConnected:  true,
	}

	return connection, nil
}

// Subscribe sends LISTEN messages for the given topics with the auth token.
func (c *Connection) Subscribe(topics []*model.PubSubTopic) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, topic := range topics {
		if c.hasTopic(topic) {
			continue
		}
		c.topics = append(c.topics, topic)

		if !c.isConnected {
			c.pendingTopics = append(c.pendingTopics, topic)
			continue
		}

		if err := c.sendListen(topic); err != nil {
			return fmt.Errorf("subscribing to topic %s: %w", topic, err)
		}
	}
	return nil
}

// Unsubscribe sends UNLISTEN messages for the given topics.
func (c *Connection) Unsubscribe(topics []*model.PubSubTopic) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	topicStrings := make([]string, 0, len(topics))
	for _, topic := range topics {
		topicStrings = append(topicStrings, topic.String())
	}

	nonce := auth.GenerateHex(16)
	req := Request{
		Type:  TypeUnlisten,
		Nonce: nonce,
		Data: &RequestData{
			Topics:    topicStrings,
			AuthToken: c.auth.AuthToken(),
		},
	}

	if err := c.sendRequest(req); err != nil {
		c.log.Error("Failed to unlisten from topics",
			"conn", c.index, "topics", topicStrings, "error", err)
		return err
	}

	for _, topic := range topics {
		c.removeTopic(topic)
	}

	c.log.Debug("Unlistened from topics", "conn", c.index, "topics", topicStrings)
	return nil
}

// Run starts the read loop, write loop, and ping loop for this connection.
// It blocks until the context is cancelled or a fatal error occurs.
func (c *Connection) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go c.writeLoop(ctx)

	c.enqueuePing()

	c.mu.Lock()
	for _, topic := range c.pendingTopics {
		if err := c.sendListen(topic); err != nil {
			c.log.Error("Failed to subscribe pending topic",
				"conn", c.index, "topic", topic, "error", err)
		}
	}
	c.pendingTopics = nil
	c.mu.Unlock()

	go c.pingLoop(ctx)

	return c.readLoop(ctx)
}

// Close gracefully closes the WebSocket connection.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isConnected = false
	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "closing")
	}
	close(c.messages)
}

// Messages returns the channel on which parsed PubSub messages are delivered.
func (c *Connection) Messages() <-chan *model.Message {
	return c.messages
}

// TopicCount returns the number of currently subscribed topics.
func (c *Connection) TopicCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.topics)
}

// HasCapacity returns true if the connection can accept more topics.
func (c *Connection) HasCapacity() bool {
	return c.TopicCount() < constants.MaxTopicsPerConn
}

// Topics returns a copy of the currently subscribed topics.
func (c *Connection) Topics() []*model.PubSubTopic {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*model.PubSubTopic, len(c.topics))
	copy(out, c.topics)
	return out
}

// IsConnected returns whether the connection is currently active.
func (c *Connection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

func (c *Connection) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var resp Response
		err := wsjson.Read(ctx, c.conn, &resp)
		if err != nil {
			c.mu.Lock()
			c.isConnected = false
			c.mu.Unlock()

			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Error("WebSocket read error", "conn", c.index, "error", err)
			return fmt.Errorf("read error on conn #%d: %w", c.index, err)
		}

		c.handleResponse(ctx, &resp)
	}
}

func (c *Connection) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-c.writeCh:
			if !ok {
				return
			}
			err := c.conn.Write(ctx, websocket.MessageText, data)
			if err != nil {
				c.log.Error("WebSocket write error", "conn", c.index, "error", err)
			}
		}
	}
}

func (c *Connection) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(constants.DefaultPubSubPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			elapsed := time.Since(c.lastPong)
			connected := c.isConnected
			c.mu.Unlock()

			if !connected {
				return
			}

			if elapsed > 5*time.Minute {
				c.log.Warn("No PONG received in over 5 minutes, connection may be dead",
					"conn", c.index, "elapsed", elapsed.Round(time.Second))
				return
			}

			c.enqueuePing()
		}
	}
}

func (c *Connection) handleResponse(ctx context.Context, resp *Response) {
	switch resp.Type {
	case TypePong:
		c.mu.Lock()
		c.lastPong = time.Now()
		c.mu.Unlock()

	case TypeReconnect:
		c.log.Info("Reconnection requested by server", "conn", c.index)
		c.mu.Lock()
		c.isConnected = false
		c.mu.Unlock()

	case TypeResponse:
		if resp.Error != "" {
			c.mu.Lock()
			failedTopic := c.nonceToTopic[resp.Nonce]
			delete(c.nonceToTopic, resp.Nonce)
			c.mu.Unlock()

			c.log.Error("PubSub LISTEN error",
				"conn", c.index,
				"error", resp.Error,
				"topic", failedTopic,
				"nonce", resp.Nonce,
			)

			if resp.Error == "ERR_BADAUTH" {
				c.log.Error("Received ERR_BADAUTH — auth token may be expired or invalid",
					"conn", c.index)
			}
		} else {
			c.mu.Lock()
			delete(c.nonceToTopic, resp.Nonce)
			c.mu.Unlock()
		}

	case TypeMessage:
		c.handleMessage(ctx, resp.Data)
	}
}

func (c *Connection) handleMessage(ctx context.Context, rawData json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(rawData, &msgData); err != nil {
		c.log.Error("Failed to parse MESSAGE data", "conn", c.index, "error", err)
		return
	}

	msg, err := model.ParseMessage(msgData.Topic, []byte(msgData.Message))
	if err != nil {
		c.log.Error("Failed to parse PubSub message",
			"conn", c.index, "topic", msgData.Topic, "error", err)
		return
	}

	c.mu.Lock()
	if c.lastMsgIdentifier == msg.Identifier && c.lastMsgTimestamp.Equal(msg.Timestamp) {
		c.mu.Unlock()
		return
	}
	c.lastMsgTimestamp = msg.Timestamp
	c.lastMsgIdentifier = msg.Identifier
	c.mu.Unlock()

	select {
	case c.messages <- msg:
	case <-ctx.Done():
	}
}

func (c *Connection) sendListen(topic *model.PubSubTopic) error {
	nonce := auth.GenerateHex(16)
	topicStr := topic.String()
	c.nonceToTopic[nonce] = topicStr

	req := Request{
		Type:  TypeListen,
		Nonce: nonce,
		Data: &RequestData{
			Topics:    []string{topicStr},
			AuthToken: c.auth.AuthToken(),
		},
	}

	c.log.Debug("Subscribing to topic", "conn", c.index, "topic", topicStr)
	return c.sendRequest(req)
}

func (c *Connection) sendRequest(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	select {
	case c.writeCh <- data:
		return nil
	default:
		return fmt.Errorf("write channel full on conn #%d", c.index)
	}
}

func (c *Connection) enqueuePing() {
	req := Request{Type: TypePing}
	data, err := json.Marshal(req)
	if err != nil {
		c.log.Error("Failed to marshal PING", "conn", c.index, "error", err)
		return
	}

	select {
	case c.writeCh <- data:
		c.log.Debug("Sent PING", "conn", c.index)
	default:
		c.log.Warn("Write channel full, dropping PING", "conn", c.index)
	}
}

func (c *Connection) hasTopic(topic *model.PubSubTopic) bool {
	topicStr := topic.String()
	for _, t := range c.topics {
		if t.String() == topicStr {
			return true
		}
	}
	return false
}

func (c *Connection) removeTopic(topic *model.PubSubTopic) {
	topicStr := topic.String()
	for i, t := range c.topics {
		if t.String() == topicStr {
			c.topics = append(c.topics[:i], c.topics[i+1:]...)
			return
		}
	}
	for i, t := range c.pendingTopics {
		if t.String() == topicStr {
			c.pendingTopics = append(c.pendingTopics[:i], c.pendingTopics[i+1:]...)
			return
		}
	}
}
