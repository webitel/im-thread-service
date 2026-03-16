package querier

import (
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type ScanFn func(any) []any

type MessageQuerier interface {
	ScanFn() ScanFn
	Insert(message *model.Message) (string, []any)
}

type MessageButtonInteractionQuerier interface {
	ScanFn(mbi *model.MessageButtonInteraction, result model.InteractionResult) ([]any, error)
	Insert(mbi *model.MessageButtonInteraction) (string, []any, error)
}