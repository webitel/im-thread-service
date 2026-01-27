package imcontact

import (
	"context"
	"fmt"
	"log/slog"

	contactv1 "github.com/webitel/im-thread-service/gen/go/client/contact/v1"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"google.golang.org/grpc"
)

const ServiceName string = "im-contact-service"

type ContactsService interface {
	SearchContact(ctx context.Context, req *contactv1.SearchContactRequest) (*contactv1.ContactList, error)
	CreateContact(ctx context.Context, req *contactv1.CreateContactRequest) (*contactv1.Contact, error)
	UpdateContact(ctx context.Context, req *contactv1.UpdateContactRequest) (*contactv1.Contact, error)
	DeleteContact(ctx context.Context, req *contactv1.DeleteContactRequest) (*contactv1.Contact, error)
	CanSend(ctx context.Context, req *dto.CanSendRequest) (*dto.CanSendResponse, error)
}

type Client struct {
	logger *slog.Logger
	rpc    *rpc.Client[contactv1.ContactsClient]
}

func New(logger *slog.Logger, discovery discovery.DiscoveryProvider) (*Client, error) {
	factory := func(conn *grpc.ClientConn) contactv1.ContactsClient {
		return contactv1.NewContactsClient(conn)
	}

	c, err := webitel.New(discovery, ServiceName, factory)
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to initialize: %w", err)
	}

	return &Client{
		logger: logger,
		rpc:    c,
	}, nil
}

type contactsClientWithLog struct {
	logger *slog.Logger
	contactv1.ContactsClient
}

func (c *Client) SearchContact(ctx context.Context, req *contactv1.SearchContactRequest) (*contactv1.ContactList, error) {
	var resp *contactv1.ContactList
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Debug("CONTACTS.SEARCH_CONTACT", slog.Any("REQ", req))
		var err error
		resp, err = api.SearchContact(ctx, req)
		return err
	})
	return resp, err
}

func (c *Client) CreateContact(ctx context.Context, req *contactv1.CreateContactRequest) (*contactv1.Contact, error) {
	var resp *contactv1.Contact
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Info("CONTACTS.CREATE_CONTACT", slog.String("NAME", req.GetName()))
		var err error
		resp, err = api.CreateContact(ctx, req)
		return err
	})
	return resp, err
}

func (c *Client) UpdateContact(ctx context.Context, req *contactv1.UpdateContactRequest) (*contactv1.Contact, error) {
	var resp *contactv1.Contact
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Info("CONTACTS.UPDATE_CONTACT", slog.String("ID", req.GetId()))
		var err error
		resp, err = api.UpdateContact(ctx, req)
		return err
	})
	return resp, err
}

func (c *Client) DeleteContact(ctx context.Context, req *contactv1.DeleteContactRequest) (*contactv1.Contact, error) {
	var resp *contactv1.Contact
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Warn("CONTACTS.DELETE_CONTACT", slog.String("ID", req.GetId()))
		var err error
		resp, err = api.DeleteContact(ctx, req)
		return err
	})
	return resp, err
}

func (c *Client) CanSend(ctx context.Context, req *dto.CanSendRequest) (*dto.CanSendResponse, error) {
	return dto.NewCanSendResponse(true), nil

	var resp *dto.CanSendResponse
	err := c.rpc.Execute(ctx, func(api contactv1.ContactsClient) error {
		pb := &contactv1.CanSendRequest{
			DomainId: req.DomainID,
			From:     c.mapModelPeerToProto(req.From),
			To:       c.mapModelPeerToProto(req.To),
		}

		out, err := api.CanSend(ctx, pb)
		if err != nil {
			return err
		}
		resp = dto.NewCanSendResponse(out.GetCan())
		return nil
	})
	return resp, err
}

func (c *Client) mapModelPeerToProto(p model.Peer) *contactv1.CanSendRequest_Peer {
	peer := &contactv1.CanSendRequest_Peer{}

	switch p.Type {
	case model.PeerUser:
		peer.Kind = &contactv1.CanSendRequest_Peer_ContactId{
			ContactId: p.ID.String(),
		}
	case model.PeerBot:
		peer.Kind = &contactv1.CanSendRequest_Peer_BotId{
			BotId: p.ID.String(),
		}
	default:
		c.logger.Error("failed to map peer to proto: unknown peer type",
			"type", p.Type,
			"id", p.ID.String(),
		)
		peer.Kind = &contactv1.CanSendRequest_Peer_ContactId{
			ContactId: p.ID.String(),
		}
	}
	return peer
}

func (c *Client) Close() error {
	if c.rpc != nil {
		return c.rpc.Close()
	}
	return nil
}

var _ ContactsService = (*Client)(nil)
