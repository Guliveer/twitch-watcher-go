package pubsub

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// MessageHandler processes decoded PubSub messages routed from the pool.
type MessageHandler interface {
	HandlePubSubMessage(ctx context.Context, msg *model.Message)
}

// Pool manages multiple PubSub WebSocket connections, distributing topics
// across them and routing incoming messages to a handler.
type Pool struct {
	mu sync.Mutex

	conns []*Connection
	auth auth.Provider
	log *logger.Logger
	handler MessageHandler

	merged chan *model.Message

	maxTopics int
	maxConns int
}

// NewPool creates a new PubSub connection pool.
func NewPool(a auth.Provider, log *logger.Logger, handler MessageHandler) *Pool {
	return &Pool{
		conns:     make([]*Connection, 0, constants.MaxPubSubConns),
		auth:      a,
		log:       log,
		handler:   handler,
		merged:    make(chan *model.Message, 256),
		maxTopics: constants.MaxTopicsPerConn,
		maxConns:  constants.MaxPubSubConns,
	}
}

// Subscribe distributes topics across connections, creating new connections
// as needed. Each connection holds up to MaxTopicsPerConn topics.
func (p *Pool) Subscribe(ctx context.Context, topics []*model.PubSubTopic) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, topic := range topics {
		if !topic.IsUserTopic() && topic.Streamer != nil && topic.Streamer.ChannelID == "" {
			p.log.Warn("Skipping subscription for topic with empty channel_id",
				"topic", topic.TopicType.String(),
				"streamer", topic.Streamer.Username,
			)
			continue
		}

		if err := p.subscribeSingle(ctx, topic); err != nil {
			return err
		}
	}
	return nil
}

// Unsubscribe removes topics from their respective connections.
func (p *Pool) Unsubscribe(topics []*model.PubSubTopic) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, topic := range topics {
		found := false
		topicStr := topic.String()

		for _, conn := range p.conns {
			for _, ct := range conn.Topics() {
				if ct.String() == topicStr {
					if err := conn.Unsubscribe([]*model.PubSubTopic{topic}); err != nil {
						p.log.Error("Failed to unsubscribe topic",
							"topic", topicStr, "error", err)
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			p.log.Warn("Topic not found in any connection", "topic", topicStr)
		}
	}
	return nil
}

// UnsubscribeStreamer removes all topics associated with a specific streamer.
// It collects topics under lock, then calls Unsubscribe (which handles its own locking).
func (p *Pool) UnsubscribeStreamer(streamer *model.Streamer) error {
	p.mu.Lock()
	var topicsToRemove []*model.PubSubTopic
	for _, conn := range p.conns {
		for _, topic := range conn.Topics() {
			if !topic.IsUserTopic() && topic.Streamer != nil &&
				topic.Streamer.ChannelID == streamer.ChannelID {
				topicsToRemove = append(topicsToRemove, topic)
			}
		}
	}
	p.mu.Unlock()

	if len(topicsToRemove) == 0 {
		p.log.Warn("No topics found", "streamer", streamer.Username)
		return nil
	}

	p.log.Debug("Unsubscribing from streamer topics",
		"streamer", streamer.Username, "count", len(topicsToRemove))

	return p.Unsubscribe(topicsToRemove)
}

// Run starts all connections and routes messages to the handler.
// It blocks until the context is cancelled or a fatal error occurs.
// Dead connections are automatically reconnected with exponential backoff.
func (p *Pool) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return p.routeMessages(ctx)
	})

	g.Go(func() error {
		return p.healthMonitor(ctx)
	})

	p.mu.Lock()
	for _, conn := range p.conns {
		conn := conn // capture for closure
		p.startForwarder(ctx, conn)
		g.Go(func() error {
			return p.runConnection(ctx, conn)
		})
	}
	p.mu.Unlock()

	return g.Wait()
}

// Close gracefully closes all connections in the pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		conn.Close()
	}
	p.log.Info("PubSub pool closed", "connections", len(p.conns))
}

// ConnectionCount returns the number of active connections.
func (p *Pool) ConnectionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

// TotalTopicCount returns the total number of subscribed topics across all connections.
func (p *Pool) TotalTopicCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	total := 0
	for _, conn := range p.conns {
		total += conn.TopicCount()
	}
	return total
}

// subscribeSingle subscribes a single topic to an available connection,
func (p *Pool) subscribeSingle(ctx context.Context, topic *model.PubSubTopic) error {
	for _, conn := range p.conns {
		if conn.HasCapacity() {
			return conn.Subscribe([]*model.PubSubTopic{topic})
		}
	}

	if len(p.conns) >= p.maxConns {
		return fmt.Errorf("maximum number of PubSub connections (%d) reached", p.maxConns)
	}

	conn, err := NewConnection(ctx, len(p.conns), p.auth, p.log)
	if err != nil {
		return fmt.Errorf("creating new PubSub connection: %w", err)
	}

	p.conns = append(p.conns, conn)
	p.log.Info("Created new PubSub connection",
		"conn", conn.index, "total_connections", len(p.conns))

	return conn.Subscribe([]*model.PubSubTopic{topic})
}

func (p *Pool) runConnection(ctx context.Context, conn *Connection) error {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		err := conn.Run(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		p.log.Warn("PubSub connection lost, reconnecting",
			"conn", conn.index, "error", err, "backoff", backoff.Round(time.Second))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))

		if err := p.reconnect(ctx, conn); err != nil {
			p.log.Error("Reconnection failed", "conn", conn.index, "error", err)
			continue
		}

		backoff = time.Second
		p.log.Info("PubSub connection re-established", "conn", conn.index)
	}
}

func (p *Pool) reconnect(ctx context.Context, conn *Connection) error {
	topics := conn.Topics()

	newConn, err := NewConnection(ctx, conn.index, p.auth, p.log)
	if err != nil {
		return fmt.Errorf("dialing PubSub for reconnection: %w", err)
	}

	p.mu.Lock()
	for i, c := range p.conns {
		if c == conn {
			p.conns[i] = newConn
			break
		}
	}
	p.startForwarder(ctx, newConn)
	p.mu.Unlock()

	if err := newConn.Subscribe(topics); err != nil {
		return fmt.Errorf("re-subscribing topics after reconnection: %w", err)
	}

	go func() {
		if err := newConn.Run(ctx); err != nil && ctx.Err() == nil {
			p.log.Error("Reconnected connection failed", "conn", newConn.index, "error", err)
		}
	}()

	return nil
}

// startForwarder launches a goroutine that reads from a connection's messages
// channel and forwards them to the pool's merged fan-in channel.
// The goroutine exits when the connection's messages channel is closed or the
func (p *Pool) startForwarder(ctx context.Context, conn *Connection) {
	go func() {
		for {
			select {
			case msg, ok := <-conn.Messages():
				if !ok {
					return
				}
				select {
				case p.merged <- msg:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *Pool) routeMessages(ctx context.Context) error {
	for {
		select {
		case msg, ok := <-p.merged:
			if !ok {
				return nil
			}
			if p.handler != nil {
				p.handler.HandlePubSubMessage(ctx, msg)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Pool) healthMonitor(ctx context.Context) error {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.mu.Lock()
			for _, conn := range p.conns {
				if !conn.IsConnected() {
					p.log.Warn("Connection is not connected",
						"conn", conn.index, "topics", conn.TopicCount())
				}
			}
			p.mu.Unlock()
		}
	}
}
