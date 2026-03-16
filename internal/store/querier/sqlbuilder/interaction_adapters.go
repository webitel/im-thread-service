package sqlbuilder

import (
	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
)


var (
    mbiStruct      = sqlbuilder.NewStruct(new(model.MessageButtonInteraction)).For(sqlbuilder.PostgreSQL)
    postbackStruct = sqlbuilder.NewStruct(new(model.InteractionPostback)).For(sqlbuilder.PostgreSQL)
    contactStruct  = sqlbuilder.NewStruct(new(model.InteractionContact)).For(sqlbuilder.PostgreSQL)
	locationStruct = sqlbuilder.NewStruct(new(model.InteractionLocation)).For(sqlbuilder.PostgreSQL)
)

var (
	interactionHandlers = map[model.ButtonActionType]interactionSQLAdapter {
		model.ContactAction: new(contactAdapter),
		model.LocationAction: new(locationAdapter),
		model.PostbackAction: new(postbackAdapter),
	}
)

type interactionSQLAdapter interface {
    buildCTE(result model.InteractionResult) *sqlbuilder.CTEQueryBuilder
    joinAlias() string
    columns() []string
    joinCondition() string
	nameOfCTE() string
	scanFn(st any) []any
}

//#region Postback

type postbackAdapter struct{}

func (a postbackAdapter) buildCTE(result model.InteractionResult) *sqlbuilder.CTEQueryBuilder {
    r := result.(*model.InteractionPostback)

    pb := postbackStruct.InsertInto(InteractionPostbackTable).
        Cols("interaction_id", "callback_data")

    sel := pb.Select("id")
    sel.SelectMore(sel.Var(r.CallbackData))
    sel.From("interaction")

    pb.Returning(postbackStruct.Columns()...)

    return sqlbuilder.CTEQuery("postback").As(pb)
}

func (a postbackAdapter) scanFn(st any) []any { return postbackStruct.Addr(st)}

func (a postbackAdapter) nameOfCTE() string { return "postback" }

func (a postbackAdapter) joinAlias() string {
    return "p"
}

func (a postbackAdapter) columns() []string {
    return postbackStruct.Columns()
}

func (a postbackAdapter) joinCondition() string {
    return "p.interaction_id = i.id"
}

// #endregion

// #region Contact

type contactAdapter struct{}

func (c *contactAdapter) buildCTE(result model.InteractionResult) *sqlbuilder.CTEQueryBuilder {
	r := result.(*model.InteractionContact)

	cb := contactStruct.InsertInto(InteractionContactTable)
    cb.Cols(contactStruct.Columns()...)

    sel := cb.Select("id")
    sel.SelectMore(
        sel.Var(r.Name),
        sel.Var(r.PhoneNumber),
        sel.Var(r.Metadata),
    )
    sel.From("interaction")

    cb.Returning(contactStruct.Columns()...)
    return sqlbuilder.CTEQuery("contact").As(cb)
}

func (c *contactAdapter) nameOfCTE() string { return "contact" }

func (c *contactAdapter) scanFn(st any) []any { return contactStruct.Addr(st)}

func (c *contactAdapter) joinAlias() string { return "c" }

func (c *contactAdapter) columns() []string { return contactStruct.Columns() }

func (c *contactAdapter) joinCondition() string { return "c.interaction_id = i.id"}

// #endregion

// #region Location
type locationAdapter struct{}

func (l *locationAdapter) buildCTE(result model.InteractionResult) *sqlbuilder.CTEQueryBuilder {
	r := result.(*model.InteractionLocation)

	lb := locationStruct.
		InsertInto(InteractionLocationTable).
		Cols(locationStruct.Columns()...)

	sel := lb.Select("id").From("interaction")
	sel.SelectMore(
		sel.Var(r.Latitude),
		sel.Var(r.Longitude),
		sel.Var(r.City),
		sel.Var(r.State),
		sel.Var(r.Country),
		sel.Var(r.PostalCode),
	)

	lb.Returning(locationStruct.Columns()...)

	return sqlbuilder.CTEQuery("location").As(lb)
}

func (l *locationAdapter) nameOfCTE() string { return "location" }

func (l *locationAdapter) joinAlias() string { return "l" }

func (l *locationAdapter) scanFn(st any) []any { return locationStruct.Addr(st) }

func (l *locationAdapter) columns() []string { return locationStruct.Columns() }

func (l *locationAdapter) joinCondition() string { return "l.interaction_id = i.id" }
// #endregion