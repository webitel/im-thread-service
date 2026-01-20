package storage

import (
	"context"
	"log/slog"

	"github.com/webitel/im-thread-service/gen/go/client/storage"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"google.golang.org/grpc"
)

const ServiceName string = "storage"

type FileService interface {
	SearchFiles(ctx context.Context, in *storage.SearchFilesRequest) (*storage.ListFile, error)
	UploadFileUrl(ctx context.Context, in *storage.UploadFileUrlRequest) (*storage.UploadFileUrlResponse, error)
}

type Client struct {
	Logger *slog.Logger
	rpc    *rpc.Client[storage.FileServiceClient]
}

type StorageClient struct {
	logger *slog.Logger
	storage.FileServiceClient
}

func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	factory := func(conn *grpc.ClientConn) storage.FileServiceClient {
		return storage.NewFileServiceClient(conn)
	}

	c, err := webitel.New(discovery, ServiceName, factory)
	if err != nil {
		return nil, err
	}

	return &Client{
		Logger: logger,
		rpc:    c,
	}, nil
}

type storageClientWithLog struct {
	logger *slog.Logger
	storage.FileServiceClient
}

func (s *storageClientWithLog) SearchFiles(ctx context.Context, in *storage.SearchFilesRequest, opts ...grpc.CallOption) (*storage.ListFile, error) {
	s.logger.Debug("STORAGE.SEARCH_FILES", slog.Any("REQUEST", in))
	return s.FileServiceClient.SearchFiles(ctx, in, opts...)
}

func (s *storageClientWithLog) UploadFileUrl(ctx context.Context, in *storage.UploadFileUrlRequest, opts ...grpc.CallOption) (*storage.UploadFileUrlResponse, error) {
	s.logger.Info("STORAGE.UPLOAD_FILE_URL", slog.String("URL", in.GetUrl()))
	return s.FileServiceClient.UploadFileUrl(ctx, in, opts...)
}

func (c *Client) SearchFiles(ctx context.Context, in *storage.SearchFilesRequest) (*storage.ListFile, error) {
	var resp *storage.ListFile

	err := c.rpc.Execute(ctx, func(api storage.FileServiceClient) error {
		var err error
		wrapper := &storageClientWithLog{logger: c.Logger, FileServiceClient: api}
		resp, err = wrapper.SearchFiles(ctx, in)
		return err
	})

	return resp, err
}

func (c *Client) UploadFileUrl(ctx context.Context, in *storage.UploadFileUrlRequest) (*storage.UploadFileUrlResponse, error) {
	var resp *storage.UploadFileUrlResponse

	err := c.rpc.Execute(ctx, func(api storage.FileServiceClient) error {
		var err error
		wrapper := &storageClientWithLog{logger: c.Logger, FileServiceClient: api}
		resp, err = wrapper.UploadFileUrl(ctx, in)
		return err
	})

	return resp, err
}
