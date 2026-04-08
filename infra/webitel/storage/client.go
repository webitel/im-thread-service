package storageclient

import (
	"context"
	"fmt"
	"log/slog"

	storagev1 "github.com/webitel/im-thread-service/gen/go/storage"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ServiceName string = "storage"

// [CLIENT] Wrapper for Storage FileService with resilient RPC execution
type Client struct {
	logger *slog.Logger
	rpc    *rpc.Client[storagev1.FileServiceClient]
}

// New initializes the Storage client with service discovery and load balancing
func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	factory := func(conn *grpc.ClientConn) storagev1.FileServiceClient {
		return storagev1.NewFileServiceClient(conn)
	}

	c, err := webitel.New(logger, discovery, ServiceName, nil, factory)
	if err != nil {
		return nil, fmt.Errorf("[storage-client] initialization failed: %w", err)
	}

	return &Client{
		logger: logger,
		rpc:    c,
	}, nil
}

// SearchFiles performs a file lookup using the resilient Execute wrapper
func (c *Client) SearchFiles(ctx context.Context, req *storagev1.SearchFilesRequest) (*storagev1.ListFile, error) {
	var resp *storagev1.ListFile
	err := c.rpc.Execute(ctx, func(api storagev1.FileServiceClient) error {
		c.logger.Debug("STORAGE.SEARCH_FILES", slog.Any("req", req))

		var err error
		resp, err = api.SearchFiles(ctx, req)
		return err
	})

	return resp, err
}

// UploadFileUrl uploads a file from a remote URL
func (c *Client) UploadFileUrl(ctx context.Context, req *storagev1.UploadFileUrlRequest) (*storagev1.UploadFileUrlResponse, error) {
	var resp *storagev1.UploadFileUrlResponse

	err := c.rpc.Execute(ctx, func(api storagev1.FileServiceClient) error {
		c.logger.Info("STORAGE.UPLOAD_FILE_URL", slog.String("url", req.GetUrl()))

		var err error
		resp, err = api.UploadFileUrl(ctx, req)
		return err
	})

	return resp, err
}

func (c *Client) BulkGenerateFileLink(ctx context.Context, req *storagev1.BulkGenerateFileLinkRequest) (*storagev1.BulkGenerateFileLinkResponse, error) {
	var (
		resp *storagev1.BulkGenerateFileLinkResponse
		err  error
	)

	c.rpc.Execute(ctx, func(fsc storagev1.FileServiceClient) error {
		resp, err = fsc.BulkGenerateFileLink(ctx, req)
		return err
	})

	if err != nil {
		st, _ := status.FromError(err)
		errGroup := slog.Group("grpc_info",
			slog.String("error", st.Message()),
			slog.String("code", st.Code().String()),
		)

		switch st.Code() {
		case codes.Unavailable:
			c.logger.WarnContext(ctx, "STORAGE.BulkGenerateFileLink TEMPORARY UNAVAILABLE", errGroup)
		default:
			c.logger.ErrorContext(ctx, "STORAGE.BulkGenerateFileLink gRPC CALL ERROR", errGroup)
		}

		return nil, err
	}

	return resp, nil
}

// Close gracefully shuts down the underlying gRPC connection pool
func (c *Client) Close() error {
	if c.rpc != nil {
		return c.rpc.Close()
	}
	return nil
}
