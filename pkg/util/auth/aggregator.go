package auth

import (
	"context"
	"net/http"
)

type AuthenticationMode string
type contextKey string

const (
	AuthInfoContextKey contextKey = "AuthInfo"
)

type AggregatorUserInfo struct {
	User   string
	Groups []string
	Extra  map[string][]string
}

func CreateContextWithAuthInfo(ctx context.Context, info AggregatorUserInfo) context.Context {
	return context.WithValue(ctx, AuthInfoContextKey, info)
}

func RetrieveAuthInfoFromContext(ctx context.Context) (AggregatorUserInfo, bool) {
	authInfo, ok := ctx.Value(AuthInfoContextKey).(AggregatorUserInfo)
	return authInfo, ok
}

func RequestUser(r *http.Request) (User, error) {
	ctx := r.Context()
	authInfo, ok := RetrieveAuthInfoFromContext(ctx)
	if !ok {
		return nil, ErrAuthInfoMissing(authInfo)
	}
	user, err := Named(authInfo.User)
	if err != nil {
		return nil, ErrAuthInfoUsernameIsMissing(authInfo)
	}
	return user, nil
}

func MaybeRequestUser(r *http.Request) OptionalUser {
	ctx := r.Context()
	authInfo, ok := RetrieveAuthInfoFromContext(ctx)
	if !ok {
		return NoUser()
	}
	return MaybeNamed(authInfo.User)
}
