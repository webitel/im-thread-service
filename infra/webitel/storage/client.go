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
)

const ServiceName string = "storage"

// [CLIENT] Wrapper for Storage FileService with resilient RPC execution
type Client struct {
	logger *slog.Logger
	// [GENERIC_RPC] Holds the go-kit RPC client specifically for FileService
	rpc *rpc.Client[storagev1.FileServiceClient]
}

// New initializes the Storage client with service discovery and load balancing
func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	// [FACTORY] Instantiates the gRPC stub for FileServiceClient
	factory := func(conn *grpc.ClientConn) storagev1.FileServiceClient {
		return storagev1.NewFileServiceClient(conn)
	}

	// [INIT] webitel.New returns a go-kit RPC wrapper with discovery support
	c, err := webitel.New(logger, discovery, ServiceName, factory)
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

	// [EXECUTE] Handles load balancing and retries
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

// Close gracefully shuts down the underlying gRPC connection pool
func (c *Client) Close() error {
	if c.rpc != nil {
		return c.rpc.Close()
	}
	return nil
}
