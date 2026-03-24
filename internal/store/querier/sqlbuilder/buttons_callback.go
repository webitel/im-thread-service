package sqlbuilder

import (
	"fmt"

	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

var buttonsCallbackStruct = sqlbuilder.NewStruct(new(model.ButtonsCallback)).For(sqlbuilder.PostgreSQL)

func PrepareButtonsCallbackInsertQuery(callback *model.ButtonsCallback) (string, []any) {
	ibd := buttonsCallbackStruct.
		InsertInto(ButtonsCallbackTable).
		Cols(buttonsCallbackStruct.WithoutTag(IgnoreTag).Columns()...).
		Returning(buttonsCallbackStruct.Columns()...)

	sel := ibd.Select("m.id").From(
		fmt.Sprintf("%s as m", MessageTable),
	)

	sel.SelectMore(
		sel.Var(callback.ButtonCode),
		sel.Var(callback.CallbackData),
		sel.Var(callback.ClickedBy),
	).
	JoinWithOption(
		sqlbuilder.InnerJoin,
		fmt.Sprintf("%s as t", ThreadTable),
		"t.id = m.thread_id",
	).
	JoinWithOption(
		sqlbuilder.InnerJoin,
		fmt.Sprintf("%s as td", ThreadDialogTable),
		fmt.Sprintf("(td.member_id, td.thread_id) = (%s, t.id)", sel.Var(callback.ClickedBy)),
	).
	Where(
		sel.Equal("m.id", callback.MessageID),
	).
	Limit(1)
	
	return ibd.Build()
}