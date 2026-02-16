// Package pubsub implements the Twitch PubSub WebSocket protocol, providing
// a connection pool that manages multiple WebSocket connections, automatic
// topic distribution, ping/pong keepalive, and reconnection with exponential
// backoff.
package pubsub

import "encoding/json"

// PubSub protocol message types sent to/from the Twitch PubSub server.
const (
	// TypePing is sent by the client to keep the connection alive.
	TypePing = "PING"
	// TypePong is the server's response to a PING.
	TypePong = "PONG"
	// TypeListen subscribes to one or more topics.
	TypeListen = "LISTEN"
	// TypeUnlisten unsubscribes from one or more topics.
	TypeUnlisten = "UNLISTEN"
	// TypeMessage is a server-pushed message for a subscribed topic.
	TypeMessage = "MESSAGE"
	// TypeResponse is the server's acknowledgement of a LISTEN/UNLISTEN.
	TypeResponse = "RESPONSE"
	// TypeReconnect is sent by the server to request a client reconnection.
	TypeReconnect = "RECONNECT"
)

// Request is a message sent from the client to the Twitch PubSub server.
type Request struct {
	Type  string       `json:"type"`
	Nonce string       `json:"nonce,omitempty"`
	Data  *RequestData `json:"data,omitempty"`
}

// RequestData contains the topics and auth token for LISTEN/UNLISTEN requests.
type RequestData struct {
	Topics    []string `json:"topics"`
	AuthToken string   `json:"auth_token"`
}

// Response is a message received from the Twitch PubSub server.
type Response struct {
	Type  string          `json:"type"`
	Nonce string          `json:"nonce,omitempty"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// MessageData is the payload within a MESSAGE-type response.
type MessageData struct {
	Topic   string `json:"topic"`
	Message string `json:"message"`
}
