package model

import "context"

type jwtContextKey string

const JwtContextKey jwtContextKey = "jwt_payload"

func WithJWTPayload(ctx context.Context, payload string) context.Context {
	return context.WithValue(ctx, JwtContextKey, payload)
}

func TryGetPayloadFromContext(ctx context.Context) (string, bool) {
	payload, ok := ctx.Value(JwtContextKey).(string)

	return payload, ok
}
