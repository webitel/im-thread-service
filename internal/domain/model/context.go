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

type deviceContextKey string

const DeviceContextKey deviceContextKey = "device_id"

func WithDeviceID(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, DeviceContextKey, deviceID)
}

func TryGetDeviceIDFromContext(ctx context.Context) (string, bool) {
	deviceID, ok := ctx.Value(DeviceContextKey).(string)

	return deviceID, ok
}

type Metadater interface {
	AddMetadata(key, value string)
}

func WithContextPropogatedMetadata(ctx context.Context, metadataProvider Metadater) {
	if payload, ok := TryGetPayloadFromContext(ctx); ok {
		metadataProvider.AddMetadata(XJWTPayload, payload)
	}

	if device, ok := TryGetDeviceIDFromContext(ctx); ok {
		metadataProvider.AddMetadata(XWebitelDevice, device)
	}
}
