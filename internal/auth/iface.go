package auth

import "context"

// Provider is the authentication interface used by GQL and PubSub clients.
// *Authenticator satisfies this interface.
type Provider interface {
	Login(ctx context.Context) error
	AuthToken() string
	UserID() string
	GetAuthHeaders() map[string]string
	FetchIntegrityToken(ctx context.Context) (string, error)
}
