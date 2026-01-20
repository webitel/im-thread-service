package webiteldi

import (
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	st "github.com/webitel/im-thread-service/infra/webitel/storage"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"webitel_clients",

	// fx.Provide(webitel.New),
	fx.Provide(imcontact.New),
	fx.Provide(st.New),

	fx.Provide(func(client *imcontact.Client) imcontact.ContactsService {
		return client
	}),

	fx.Provide(func(client *st.Client) st.FileService {
		return client
	}),
)
