package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

const XWebitelDeviceHeader string = "x-webitel-device"

func DeviceInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if devices := md.Get(XWebitelDeviceHeader); len(devices) > 0 {
				ctx = model.WithDeviceID(ctx, devices[0])
			}
		}

		return handler(ctx, req)
	}
}
