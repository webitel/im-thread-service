package imcontact

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"github.com/webitel/webitel-go-kit/pkg/cache"

	contactv1 "github.com/webitel/im-thread-service/gen/go/contact/v1"
	infratls "github.com/webitel/im-thread-service/infra/tls"
	"github.com/webitel/im-thread-service/infra/webitel"
)

const (
	isBotCacheTTL  = 5 * time.Minute
	isBotCacheSize = 2048
	subCacheTTL    = 5 * time.Minute
	subCacheSize   = 2048

	identityCacheTTL  = 5 * time.Minute
	identityCacheSize = 2048
)

// Identity is the subset of a contact an event has to carry so consumers do not
// have to resolve it themselves.
type Identity struct {
	Sub      string
	Issuer   string
	Type     string
	Name     string
	Username string
	IsBot    bool
}

// identityCached wraps a nullable Identity so a cached "contact not found" is
// not mistaken for a Ristretto miss.
type identityCached struct {
	identity *Identity
}

// subCached wraps a nullable int64 subscription ID.
// Stored as a struct to allow caching the "no subscription" state (sub == nil)
// without ambiguity with a Ristretto cache miss (which also returns zero value).
type subCached struct {
	sub *int64
}

const ServiceName string = "im-contact-service"

type Client struct {
	logger         *slog.Logger
	privacyService *rpc.Client[contactv1.ContactPrivacyClient]
	contactService *rpc.Client[contactv1.ContactsClient]
	isBotCache     cache.Cache[string, bool]
	subCache       cache.Cache[string, subCached]
	identityCache  cache.Cache[string, identityCached]
}

func New(logger *slog.Logger, discovery discovery.DiscoveryProvider, tlsConf *infratls.Config) (*Client, error) {
	privacyClient, err := webitel.New(logger, discovery, ServiceName, tlsConf, func(cc *grpc.ClientConn) contactv1.ContactPrivacyClient { return contactv1.NewContactPrivacyClient(cc) })
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] initialization failed: %w", err)
	}

	contactClient, err := webitel.New(logger, discovery, ServiceName, tlsConf, func(cc *grpc.ClientConn) contactv1.ContactsClient { return contactv1.NewContactsClient(cc) })
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] initialization failed: %w", err)
	}

	isBotCache, err := cache.New[string, bool]().
		L1(cache.RistrettoConfig{
			MaxCost:     isBotCacheSize,
			NumCounters: isBotCacheSize * 10,
			TTL:         isBotCacheTTL,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to create is_bot cache: %w", err)
	}

	subCache, err := cache.New[string, subCached]().
		L1(cache.RistrettoConfig{
			MaxCost:     subCacheSize,
			NumCounters: subCacheSize * 10,
			TTL:         subCacheTTL,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to create sub cache: %w", err)
	}

	identityCache, err := cache.New[string, identityCached]().
		L1(cache.RistrettoConfig{
			MaxCost:     identityCacheSize,
			NumCounters: identityCacheSize * 10,
			TTL:         identityCacheTTL,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to create identity cache: %w", err)
	}

	return &Client{
		logger:         logger,
		privacyService: privacyClient,
		contactService: contactClient,
		isBotCache:     isBotCache,
		subCache:       subCache,
		identityCache:  identityCache,
	}, nil
}

func (c *Client) CanSend(ctx context.Context, fromID, toID uuid.UUID) error {
	return c.privacyService.Execute(ctx, func(api contactv1.ContactPrivacyClient) error {
		pb := &contactv1.CanSendRequest{
			From: fromID.String(),
			To:   toID.String(),
		}

		c.logger.Debug("check permission to send messages", slog.Any("from", fromID), slog.Any("to", toID))

		var err error

		_, err = api.CanSend(ctx, pb)

		return err
	})
}

func (c *Client) CanInvite(ctx context.Context, fromID, toID uuid.UUID) error {
	return c.privacyService.Execute(ctx, func(api contactv1.ContactPrivacyClient) error {
		pb := &contactv1.CanInviteRequest{
			From: fromID.String(),
			To:   toID.String(),
		}

		c.logger.Debug("check permission to invite contact", slog.Any("from", fromID), slog.Any("to", toID))

		var err error

		_, err = api.CanInvite(ctx, pb)

		return err
	})
}

func (c *Client) SearchContact(ctx context.Context, req *contactv1.SearchContactRequest) (*contactv1.ContactList, error) {
	var resp *contactv1.ContactList

	err := c.contactService.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Debug("CONTACTS.SEARCH_CONTACT", slog.Any("req", req))

		var err error

		resp, err = api.SearchContact(ctx, req)

		return err
	})

	return resp, err
}

func (c *Client) IsBot(ctx context.Context, contactID uuid.UUID, domainID int) (bool, error) {
	key := contactID.String()

	if cached, ok, _ := c.isBotCache.Get(ctx, key); ok {
		return cached, nil
	}

	var isBot bool

	err := c.contactService.Execute(ctx, func(api contactv1.ContactsClient) error {
		c.logger.Debug("locate contact for is_bot check", slog.String("contact_id", key), slog.Int("domain_id", domainID))

		resp, err := api.Locate(ctx, &contactv1.LocateContactRequest{
			Id:       key,
			DomainId: int64(domainID),
		})
		if err != nil {
			c.logger.Debug("locate contact failed", slog.String("contact_id", key), slog.Any("err", err))

			return err
		}

		if resp.GetItem() != nil {
			isBot = resp.GetItem().GetIsBot()
		}

		c.logger.Debug("locate contact result", slog.String("contact_id", key), slog.Bool("is_bot", isBot))

		return nil
	})
	if err != nil {
		return false, err
	}

	_ = c.isBotCache.Set(ctx, key, isBot)

	return isBot, nil
}

func (c *Client) GetSub(ctx context.Context, contactID uuid.UUID, domainID int) (*int64, error) {
	key := contactID.String()

	if cached, ok, _ := c.subCache.Get(ctx, key); ok {
		return cached.sub, nil
	}

	resp, err := c.SearchContact(ctx, &contactv1.SearchContactRequest{
		Ids:      []string{key},
		DomainId: int32(domainID),
		Size:     1,
	})
	if err != nil {
		return nil, err
	}

	var sub *int64

	items := resp.GetContacts()
	if len(items) > 0 && items[0].GetSubject() != "" {
		id, err := strconv.ParseInt(items[0].GetSubject(), 10, 64)
		if err != nil {
			c.logger.Warn("contact subject is not a valid sub id", slog.String("contact_id", key), slog.String("subject", items[0].GetSubject()))
		} else {
			sub = &id
		}
	}

	_ = c.subCache.Set(ctx, key, subCached{sub: sub})

	return sub, nil
}

func (c *Client) GetIdentity(ctx context.Context, contactID uuid.UUID, domainID int) (*Identity, error) {
	key := contactID.String()

	if cached, ok, _ := c.identityCache.Get(ctx, key); ok {
		return cached.identity, nil
	}

	resp, err := c.SearchContact(ctx, &contactv1.SearchContactRequest{
		Fields:   []string{"id", "issuer_id", "type", "subject_id", "username", "name", "is_bot"},
		Ids:      []string{key},
		DomainId: int32(domainID),
		Size:     1,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve contact %s identity: %w", key, err)
	}

	var identity *Identity

	if items := resp.GetContacts(); len(items) > 0 {
		item := items[0]
		identity = &Identity{
			Sub:      item.GetSubject(),
			Issuer:   item.GetIssId(),
			Type:     item.GetType(),
			Name:     cmp.Or(item.GetName(), item.GetUsername()),
			Username: item.GetUsername(),
			IsBot:    item.GetIsBot(),
		}
	}

	_ = c.identityCache.Set(ctx, key, identityCached{identity: identity})

	return identity, nil
}

func (c *Client) Close() error {
	var errs []error

	if c.contactService != nil {
		if err := c.contactService.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing contact service client: %w", err))
		}
	}

	if c.privacyService != nil {
		if err := c.privacyService.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing privacy service client: %w", err))
		}
	}

	if c.isBotCache != nil {
		_ = c.isBotCache.Close()
	}

	if c.subCache != nil {
		_ = c.subCache.Close()
	}

	if c.identityCache != nil {
		_ = c.identityCache.Close()
	}

	return errors.Join(errs...)
}
