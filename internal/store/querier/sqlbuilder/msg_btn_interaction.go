package sqlbuilder

import (
	"fmt"

	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)


type messageButtonInteraction struct{}

func NewMessageButtonInteraction() *messageButtonInteraction {
    return &messageButtonInteraction{}
}

func (m *messageButtonInteraction) ScanFn(mbi *model.MessageButtonInteraction, result model.InteractionResult) ([]any, error) {
	addHandler, ok := interactionHandlers[result.Type()]
    if !ok {
        return nil, errors.InvalidArgument("unsupported interaction type")
    }

    addrs := mbiStruct.Addr(mbi)

    addrs = append(
        addrs,
        addHandler.scanFn(result)...,
    )

    return addrs, nil
}

func (m *messageButtonInteraction) Insert(mbi *model.MessageButtonInteraction) (string, []any, error) {
    mbiIB := mbiStruct.WithoutTag(IgnoreTag).
        InsertInto(MessageButtonInteractionTable, mbi).
        Returning(mbiStruct.Columns()...)

    isb := mbiStruct.SelectFrom("interaction as i").
		Select(addAliasToCols("i", mbiStruct.Columns()...)...)
    
	handler, ok := interactionHandlers[mbi.Action] 
	if !ok {
		return "", nil, errors.InvalidArgument("unsupported interaction type")
	}

	ctes := []*sqlbuilder.CTEQueryBuilder{
		sqlbuilder.CTEQuery("interaction").As(mbiIB),
		handler.buildCTE(mbi.Result),
	}
	
    isb.With(
		sqlbuilder.With(ctes...),
	)

	isb.Join(
		fmt.Sprintf("%s as %s", handler.nameOfCTE(), handler.joinAlias()),
		handler.joinCondition(),
	)
	
	isb.SelectMore(
		addAliasToCols(handler.joinAlias(), handler.columns()...)...
	)

    query, args := isb.Build()

    return query, args, nil
}
