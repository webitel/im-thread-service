package imcontact

import (
	"context"
	"log/slog"

	contactv1 "github.com/webitel/im-thread-service/gen/go/client/contact/v1"
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
		FromId: req.FromID,
		ToId:   req.ToID,
	}

	resp, err := c.cc.CanSend(ctx, pb)
	if err != nil {
		return nil, err
	}

	return dto.NewCanSendResponse(resp.GetCan()), nil
}
