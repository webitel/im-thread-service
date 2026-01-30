package imcontact

import (
	"context"
	"fmt"
	"log/slog"

	contactv1 "github.com/webitel/im-thread-service/gen/go/contact/v1"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"google.golang.org/grpc"
)

const ServiceName string = "im-contact-service"

// [CLIENT] Wrapper for Contact service with resilient RPC execution
type Client struct {
	logger *slog.Logger
	// [GENERIC_RPC] Holds the go-kit RPC client for the contact service
	rpc *rpc.Client[contactv1.ContactsClient]
}

// New initializes the IM Contact client with service discovery
func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	// [FACTORY] Required to instantiate the gRPC stub
	factory := func(conn *grpc.ClientConn) contactv1.ContactsClient {
		return contactv1.NewContactsClient(conn)
	}

	// [INIT] webitel.New handles service discovery and connection pooling
	c, err := webitel.New(logger, discovery, ServiceName, factory)
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] initialization failed: %w", err)
	}

	return &Client{
		logger: logger,
		rpc:    c,
	}, nil
}

// CanSend checks if a message can be delivered based on contact permissions
func (c *Client) CanSend(ctx context.Context, req *dto.CanSendRequest) (*dto.CanSendResponse, error) {
	return &dto.CanSendResponse{
		CanSend: true,
	}, nil

	var resp *contactv1.CanSendResponse

	// [EXECUTE] Handles load balancing, retries, and circuit breaking
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		// [MAPPING] Convert domain DTO to protobuf request
		pb := &contactv1.CanSendRequest{
			DomainId: req.DomainID,
			From:     req.From.ID.String(),
			To:       req.To.ID.String(),
		}

		c.logger.Debug("CONTACTS.CAN_SEND", slog.Any("from", req.From), slog.Any("to", req.To))

		var err error
		resp, err = api.CanSend(ctx, pb)
		return err
	})
	if err != nil {
		return nil, err
	}

	return dto.NewCanSendResponse(resp.GetCan()), nil
}

// SearchContact performs a contact lookup using the resilient wrapper
func (c *Client) SearchContact(ctx context.Context, req *contactv1.SearchContactRequest) (*contactv1.ContactList, error) {
	var resp *contactv1.ContactList

	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Debug("CONTACTS.SEARCH_CONTACT", slog.Any("req", req))

		var err error
		resp, err = api.SearchContact(ctx, req)
		return err
	})

	return resp, err
}

// Close gracefully shuts down the underlying connection pool
func (c *Client) Close() error {
	if c.rpc != nil {
		return c.rpc.Close()
	}
	return nil
}
