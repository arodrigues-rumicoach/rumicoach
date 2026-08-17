package auth

import "context"

// contextKey is an unexported type for request-context keys set by the auth
// middleware. Using a private type (rather than a bare string) makes these keys
// impossible for any other package to collide with — notably isAdminKey, which
// backs an authorization decision. Callers read the values through the exported
// accessors below.
type contextKey string

const (
	userIDKey  contextKey = "userID"
	emailKey   contextKey = "email"
	isAdminKey contextKey = "isAdmin"
)

// withIdentity returns a copy of ctx carrying the authenticated caller's identity.
func withIdentity(ctx context.Context, userID, email string, isAdmin bool) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, emailKey, email)
	ctx = context.WithValue(ctx, isAdminKey, isAdmin)
	return ctx
}

// WithUserID returns a copy of ctx carrying userID as the authenticated user.
// Exported for callers that populate the identity outside the HTTP middleware
// (e.g. tests), so they use the same private key the accessors read.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserID returns the authenticated user's ID and whether one is present.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

// Email returns the authenticated user's email and whether one is present.
func Email(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(emailKey).(string)
	return email, ok
}

// IsAdmin reports whether the authenticated caller has admin privileges.
func IsAdmin(ctx context.Context) bool {
	admin, _ := ctx.Value(isAdminKey).(bool)
	return admin
}
