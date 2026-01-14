package webiteldi

import (
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/infra/webitel/storage"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"webitel_clients",

	// fx.Provide(webitel.New),
	fx.Provide(imcontact.New),
	fx.Provide(storage.New),

	fx.Provide(func(client *imcontact.Client) *imcontact.ContactsClient {
		return client.ContactsService()
	}),

	fx.Provide(func(client *storage.Client) *storage.StorageClient {
		return client.FileService()
	}),
)
