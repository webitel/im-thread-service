// Package contacts is the client of the Webitel CRM contacts service
// (webitel.go, Consul name go.webitel.app). It uses the internal no-auth RPCs:
// identification runs from schemas and system flows with no user session, so
// every call is scoped by domain_id and callers must check the thread's domain.
package contacts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"

	crmpb "github.com/webitel/im-thread-service/gen/go/webitel/contacts"
	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

// ServiceName is the Consul name of webitel.go, which serves the contacts API
// over plaintext gRPC inside the cluster.
const ServiceName = "go.webitel.app"

// searchIn is how each search field is asked for: the CRM fields the term is
// matched in and the contact fields read back. Lists nested in a contact are
// cut to 16 rows by default; 64 is the most the CRM returns.
var searchIn = map[model.CRMSearchField]struct {
	qin    string
	fields []string
}{
	model.CRMSearchIMContact: {qin: "imclients{im_contact_id}", fields: []string{"id"}},
	model.CRMSearchPhones:    {qin: "phones", fields: []string{"id", "phones.size(64){number,primary}"}},
	model.CRMSearchEmails:    {qin: "emails", fields: []string{"id", "emails.size(64){email}"}},
	model.CRMSearchVariables: {qin: "variables", fields: []string{"id", "variables.size(64){key,value}"}},
}

type Client struct {
	logger    *slog.Logger
	contacts  *rpc.Client[crmpb.ContactsClient]
	imclients *rpc.Client[crmpb.IMClientsClient]
}

func New(logger *slog.Logger, dp discovery.DiscoveryProvider) (*Client, error) {
	contactsClient, err := webitel.New(logger, dp, ServiceName, nil, func(cc *grpc.ClientConn) crmpb.ContactsClient { return crmpb.NewContactsClient(cc) })
	if err != nil {
		return nil, fmt.Errorf("[crm-contacts-client] initialization failed: %w", err)
	}

	imclientsClient, err := webitel.New(logger, dp, ServiceName, nil, func(cc *grpc.ClientConn) crmpb.IMClientsClient { return crmpb.NewIMClientsClient(cc) })
	if err != nil {
		return nil, fmt.Errorf("[crm-contacts-client] initialization failed: %w", err)
	}

	return &Client{logger: logger, contacts: contactsClient, imclients: imclientsClient}, nil
}

// SearchContacts runs one CRM contact search in a domain. The search term is
// not logged: it may hold a client's phone or email.
func (c *Client) SearchContacts(ctx context.Context, q model.CRMContactQuery) ([]*model.CRMContact, bool, error) {
	req, err := searchRequest(q)
	if err != nil {
		return nil, false, err
	}

	var resp *crmpb.ContactList

	err = c.contacts.Execute(ctx, func(api crmpb.ContactsClient) error {
		var err error

		resp, err = api.SearchContactsNA(ctx, req)

		return err
	})
	if err != nil {
		return nil, false, fmt.Errorf("crm contacts search in %s: %w", req.GetQin()[0], err)
	}

	out := make([]*model.CRMContact, 0, len(resp.GetData()))
	for _, pc := range resp.GetData() {
		out = append(out, toModel(pc))
	}

	return out, resp.GetNext(), nil
}

// searchRequest builds the CRM request of one search page.
func searchRequest(q model.CRMContactQuery) (*crmpb.SearchContactsNARequest, error) {
	in, ok := searchIn[q.Field]
	if !ok {
		return nil, fmt.Errorf("crm contacts search: unknown field %d", q.Field)
	}

	if q.DomainID <= 0 {
		return nil, errors.New("crm contacts search: domain is required")
	}

	return &crmpb.SearchContactsNARequest{
		DomainId: int64(q.DomainID),
		Q:        q.Term,
		Qin:      []string{in.qin},
		Fields:   in.fields,
		Size:     q.Size,
		Page:     max(q.Page, 1),
	}, nil
}

// LinkIMContact adds an IM contact to a CRM contact. It only inserts: when a
// CRM contact of the domain already holds the IM contact the CRM rejects the
// call (model.ErrIMContactLinked), so an existing link is never moved.
func (c *Client) LinkIMContact(ctx context.Context, link model.IMContactLink) error {
	if link.DomainID <= 0 || link.CRMContactID == "" || link.IMContactID == uuid.Nil {
		return errors.New("crm link im contact: domain, contact and im contact are required")
	}

	input := []*crmpb.InputIMClient{{
		ImContactId: link.IMContactID.String(),
		Protocol:    link.Protocol,
		Via:         link.Via,
	}}

	return c.imclients.Execute(ctx, func(api crmpb.IMClientsClient) error {
		_, err := api.CreateIMClients(ctx, &crmpb.CreateIMClientsRequest{ContactId: link.CRMContactID, DomainId: int64(link.DomainID), Input: input})

		return linkError(link.CRMContactID, err)
	})
}

// linkError tells a link refused because the IM contact is already linked
// (the CRM's unique link, answered as AlreadyExists) from other failures.
func linkError(crmContactID string, err error) error {
	switch {
	case err == nil:
		return nil
	case status.Code(err) == codes.AlreadyExists:
		return fmt.Errorf("crm link im contact to %s: %w: %w", crmContactID, model.ErrIMContactLinked, err)
	default:
		return fmt.Errorf("crm link im contact to %s: %w", crmContactID, err)
	}
}

func (c *Client) Close() error {
	return errors.Join(c.contacts.Close(), c.imclients.Close())
}

func toModel(pc *crmpb.Contact) *model.CRMContact {
	out := &model.CRMContact{ID: pc.GetId()}

	for _, p := range pc.GetPhones().GetData() {
		out.Phones = append(out.Phones, model.CRMPhone{Number: p.GetNumber(), Primary: p.GetPrimary()})
	}

	for _, e := range pc.GetEmails().GetData() {
		out.Emails = append(out.Emails, e.GetEmail())
	}

	for _, v := range pc.GetVariables().GetData() {
		if out.Variables == nil {
			out.Variables = make(map[string]string)
		}

		out.Variables[v.GetKey()] = variableString(v)
	}

	for _, im := range pc.GetImclients().GetData() {
		if id := im.GetImContactId(); id != "" {
			out.IMContactIDs = append(out.IMContactIDs, id)
		}
	}

	return out
}

// variableString renders scalar variable values the way a schema sends them;
// lists and objects never match a criterion.
func variableString(v *crmpb.Variable) string {
	switch kind := v.GetValue().GetKind().(type) {
	case *structpb.Value_StringValue:
		return kind.StringValue
	case *structpb.Value_NumberValue:
		return strconv.FormatFloat(kind.NumberValue, 'f', -1, 64)
	case *structpb.Value_BoolValue:
		return strconv.FormatBool(kind.BoolValue)
	default:
		return ""
	}
}
