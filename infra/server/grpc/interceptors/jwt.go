package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

const XJWTPayloadHeader string = "x-jwt-payload"

func NewUnaryJWTInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if jwtValues := md.Get(XJWTPayloadHeader); len(jwtValues) > 0 {
				ctx = model.WithJWTPayload(ctx, jwtValues[0])
			}
		}

		return handler(ctx, req)
	}
}
