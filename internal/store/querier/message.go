package querier

import (
	"github.com/webitel/im-thread-service/internal/domain/model"
)


type MessageQuerier interface {
	ScanFn(message *model.Message) []any
	Insert(message *model.Message) (string, []any)
	InsertLocation(message *model.Message) (string, []any)
	LocationScanFn(msg *model.Message, location *model.Location) []any
	InsertContact(message *model.Message) (string, []any)
	ContactScanFn(message *model.Message) []any
}

type ButtonsCallback interface {
	Insert(callback *model.ButtonsCallback) (string, []any)
}