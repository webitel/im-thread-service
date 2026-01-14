package storage

import (
	"context"
	"log/slog"

	"github.com/webitel/im-thread-service/gen/go/client/storage"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"google.golang.org/grpc"
)

const ServiceName string = "storage"

// [CLIENT] REPRESENTS THE BASE STRUCTURE FOR STORAGE COMMUNICATION
type Client struct {
	Logger *slog.Logger
	Conn   *grpc.ClientConn
}

// [STORAGE_CLIENT] WRAPS THE GRPC SERVICE CLIENT WITH LOGGING CAPABILITIES
// WE EMBED [storage.FileServiceClient] TO AUTOMATICALLY IMPLEMENT ALL INTERFACE METHODS
type StorageClient struct {
	logger *slog.Logger
	// [EMBEDDED] ORIGINAL GRPC CLIENT TO HANDLE UNIMPLEMENTED METHODS AUTOMATICALLY
	storage.FileServiceClient
}

// [NEW] INITIALIZES A NEW BASE STORAGE CLIENT
func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	// conn, err := webitel.New(logger, discovery, ServiceName)
	conn, err := webitel.New(logger, discovery, "storage")
	if err != nil {
		return nil, err
	}

	return &Client{
		Logger: logger,
		Conn:   conn,
	}, nil
}

// [FILE_SERVICE] RETURNS AN INSTANCE OF THE FILE SERVICE CLIENT
func (c *Client) FileService() *StorageClient {
	return &StorageClient{
		logger:            c.Logger,
		FileServiceClient: storage.NewFileServiceClient(c.Conn),
	}
}

// --- OVERRIDDEN METHODS WITH LOGGING ---

// [SEARCH_FILES] SEARCHES FOR FILES IN THE STORAGE SERVICE
func (s *StorageClient) SearchFiles(ctx context.Context, in *storage.SearchFilesRequest, opts ...grpc.CallOption) (*storage.ListFile, error) {
	s.logger.Debug("STORAGE.SEARCH_FILES", slog.Any("REQUEST", in))
	return s.FileServiceClient.SearchFiles(ctx, in, opts...)
}

// [UPLOAD_FILE_URL] UPLOADS A FILE TO STORAGE USING A PROVIDED URL
func (s *StorageClient) UploadFileUrl(ctx context.Context, in *storage.UploadFileUrlRequest, opts ...grpc.CallOption) (*storage.UploadFileUrlResponse, error) {
	s.logger.Info("STORAGE.UPLOAD_FILE_URL", slog.String("URL", in.GetUrl()))
	return s.FileServiceClient.UploadFileUrl(ctx, in, opts...)
}

// [INTERFACE_GUARD] ENSURES THAT STORAGE_CLIENT FULLY IMPLEMENTS THE GRPC INTERFACE
var _ storage.FileServiceClient = (*StorageClient)(nil)
