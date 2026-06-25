package imcontact

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru"
	"google.golang.org/grpc"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"

	contactv1 "github.com/webitel/im-thread-service/gen/go/contact/v1"
	infratls "github.com/webitel/im-thread-service/infra/tls"
	"github.com/webitel/im-thread-service/infra/webitel"
)

const (
	isBotCacheTTL  = 5 * time.Minute
	isBotCacheSize = 2048
	subCacheTTL    = 5 * time.Minute
	subCacheSize   = 2048
)

type isBotCacheEntry struct {
	isBot     bool
	expiresAt time.Time
}

type subCacheEntry struct {
	sub       *int64
	expiresAt time.Time
}

const ServiceName string = "im-contact-service"

type Client struct {
	logger         *slog.Logger
	privacyService *rpc.Client[contactv1.ContactPrivacyClient]
	contactService *rpc.Client[contactv1.ContactsClient]
	isBotCache     *lru.Cache
	subCache       *lru.Cache
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

	isBotCache, err := lru.New(isBotCacheSize)
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to create is_bot cache: %w", err)
	}

	subCache, err := lru.New(subCacheSize)
	if err != nil {
		return nil, fmt.Errorf("[im-contact-client] failed to create sub cache: %w", err)
	}

	return &Client{
		logger:         logger,
		privacyService: privacyClient,
		contactService: contactClient,
		isBotCache:     isBotCache,
		subCache:       subCache,
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

	if v, ok := c.isBotCache.Get(key); ok {
		if entry, ok := v.(isBotCacheEntry); time.Now().Before(entry.expiresAt) && ok {
			return entry.isBot, nil
		}
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

	c.isBotCache.Add(key, isBotCacheEntry{isBot: isBot, expiresAt: time.Now().Add(isBotCacheTTL)})

	return isBot, nil
}

func (c *Client) GetSub(ctx context.Context, contactID uuid.UUID, domainID int) (*int64, error) {
	key := contactID.String()

	if v, ok := c.subCache.Get(key); ok {
		if entry, ok := v.(subCacheEntry); time.Now().Before(entry.expiresAt) && ok {
			return entry.sub, nil
		}
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

	c.subCache.Add(key, subCacheEntry{sub: sub, expiresAt: time.Now().Add(subCacheTTL)})

	return sub, nil
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

	return errors.Join(errs...)
}
