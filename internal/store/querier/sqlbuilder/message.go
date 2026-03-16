package sqlbuilder

import (
	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store/querier"
)

var (
	_ querier.MessageQuerier = (*messageQuerier)(nil)
)

type messageQuerier struct {
	message *sqlbuilder.Struct
}

func NewMessageQuerier() *messageQuerier {
	m := sqlbuilder.NewStruct(new(model.Message))
	m.Flavor = sqlbuilder.PostgreSQL

	return &messageQuerier{
		message: m,
	}
}

func (m *messageQuerier) ScanFn() querier.ScanFn {
	return m.message.Addr
}


func (m *messageQuerier) Insert(message *model.Message) (string, []any) {
	ib := m.message.
		WithoutTag(IgnoreTag).
		InsertInto(MessageTable, message).
		Returning(m.message.Columns()...)

	return ib.Build()
}

