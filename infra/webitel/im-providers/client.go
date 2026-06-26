package improviders

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	infratls "github.com/webitel/im-thread-service/infra/tls"
	"github.com/webitel/im-thread-service/infra/webitel"
)

const ServiceName string = "im-providers-service"

type Client struct {
	providerMessageServiceClient *rpc.Client[provider.ProviderMessageServiceClient]
}

func NewProvidersClient(logger *slog.Logger, discovery discovery.DiscoveryProvider, tlsConf *infratls.Config) (*Client, error) {
	providerMessageServiceClient, err := webitel.New(
		logger,
		discovery,
		ServiceName,
		tlsConf,
		func(cc *grpc.ClientConn) provider.ProviderMessageServiceClient {
			return provider.NewProviderMessageServiceClient(cc)
		},
	)
	if err != nil {
		return nil, errors.Internal("creating provider message service client", errors.WithCause(err), errors.WithID("improviders.client.new_providers_service_client"))
	}

	return &Client{
		providerMessageServiceClient: providerMessageServiceClient,
	}, nil
}

func (client *Client) SendText(ctx context.Context, in *provider.ProviderSendTextRequest) (*provider.ProviderSendMessageResponse, error) {
	var response *provider.ProviderSendMessageResponse

	err := client.providerMessageServiceClient.Execute(ctx, func(pmsc provider.ProviderMessageServiceClient) error {
		r, err := pmsc.SendText(ctx, in)
		if err != nil {
			return err
		}

		response = r

		return nil
	})

	return response, err
}

func (client *Client) SendDocument(ctx context.Context, in *provider.ProviderSendDocumentRequest) (*provider.ProviderSendMessageResponse, error) {
	var response *provider.ProviderSendMessageResponse

	err := client.providerMessageServiceClient.Execute(ctx, func(pmsc provider.ProviderMessageServiceClient) error {
		r, err := pmsc.SendDocument(ctx, in)
		if err != nil {
			return err
		}

		response = r

		return nil
	})

	return response, err
}

func (client *Client) SendImage(ctx context.Context, in *provider.ProviderSendImageRequest) (*provider.ProviderSendMessageResponse, error) {
	var response *provider.ProviderSendMessageResponse

	err := client.providerMessageServiceClient.Execute(ctx, func(pmsc provider.ProviderMessageServiceClient) error {
		r, err := pmsc.SendImage(ctx, in)
		if err != nil {
			return err
		}

		response = r

		return nil
	})

	return response, err
}

func (client *Client) SendInteractive(ctx context.Context, in *provider.ProviderSendInteractiveRequest) (*provider.ProviderSendMessageResponse, error) {
	var response *provider.ProviderSendMessageResponse

	err := client.providerMessageServiceClient.Execute(ctx, func(pmsc provider.ProviderMessageServiceClient) error {
		r, err := pmsc.SendInteractive(ctx, in)
		if err != nil {
			return err
		}

		response = r

		return nil
	})

	return response, err
}

func (client *Client) SendSystemMessage(ctx context.Context, in *provider.ProviderSendSystemMessageRequest) (*provider.ProviderSendMessageResponse, error) {
	var response *provider.ProviderSendMessageResponse

	err := client.providerMessageServiceClient.Execute(ctx, func(pmsc provider.ProviderMessageServiceClient) error {
		r, err := pmsc.SendSystemMessage(ctx, in)
		if err != nil {
			return err
		}

		response = r

		return nil
	})

	return response, err
}

func (client *Client) Close() error {
	if client.providerMessageServiceClient != nil {
		return client.providerMessageServiceClient.Close()
	}

	return nil
}
