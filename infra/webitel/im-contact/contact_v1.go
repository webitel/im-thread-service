package imcontact

import (
	"context"
	"log/slog"

	contactv1 "github.com/webitel/im-thread-service/gen/go/client/contact/v1"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// Go client stub for webitel.messaging.contact.v1.Contacts
type ContactsClient struct {
	logger *slog.Logger
	cc     contactv1.ContactsClient
}

func newContactsClient(client *Client) *ContactsClient {
	return &ContactsClient{
		logger: client.Logger,
		cc:     contactv1.NewContactsClient(client.Conn),
	}
}

func (c *ContactsClient) SearchContact(ctx context.Context, req *contactv1.SearchContactRequest) (*contactv1.ContactList, error) {
	return c.cc.SearchContact(ctx, req)
}

func (c *ContactsClient) CreateContact(ctx context.Context, req *contactv1.CreateContactRequest) (*contactv1.Contact, error) {
	return c.cc.CreateContact(ctx, req)
}

func (c *ContactsClient) UpdateContact(ctx context.Context, req *contactv1.UpdateContactRequest) (*contactv1.Contact, error) {
	return c.cc.UpdateContact(ctx, req)
}

func (c *ContactsClient) DeleteContact(ctx context.Context, req *contactv1.DeleteContactRequest) (*contactv1.Contact, error) {
	return c.cc.DeleteContact(ctx, req)
}

func (c *ContactsClient) CanSend(ctx context.Context, req *dto.CanSendRequest) (*dto.CanSendResponse, error) {
	pb := &contactv1.CanSendRequest{
		DomainId: req.DomainID,
		From:     c.mapModelPeerToProto(req.From),
		To:       c.mapModelPeerToProto(req.To),
	}

	resp, err := c.cc.CanSend(ctx, pb)
	if err != nil {
		return nil, err
	}

	return dto.NewCanSendResponse(resp.GetCan()), nil
}

func (c *ContactsClient) mapModelPeerToProto(p shared.Peer) *contactv1.CanSendRequest_Peer {
	peer := &contactv1.CanSendRequest_Peer{}

	switch p.Type {
	case shared.PeerContact:
		peer.Kind = &contactv1.CanSendRequest_Peer_ContactId{
			ContactId: p.ID.String(),
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
