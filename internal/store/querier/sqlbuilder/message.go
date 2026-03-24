package sqlbuilder

import (
	"fmt"
	"strings"

	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store/querier"
)

var (
	_ querier.MessageQuerier = (*messageQuerier)(nil)
)


var (
	locationStructure = sqlbuilder.NewStruct(new(model.Location)).For(sqlbuilder.PostgreSQL)
	contactStructure = sqlbuilder.NewStruct(new(model.Contact)).For(sqlbuilder.PostgreSQL)
	documentStructure = sqlbuilder.NewStruct(new(model.MessageDocument)).For(sqlbuilder.PostgreSQL)
	imageStructure = sqlbuilder.NewStruct(new(model.MessageImage)).For(sqlbuilder.PostgreSQL)
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

func (m *messageQuerier) ScanFn(message *model.Message) []any {
	return m.message.Addr(message)
}


func (m *messageQuerier) Insert(message *model.Message) (string, []any) {
	ib := m.message.
		WithoutTag(IgnoreTag).
		InsertInto(MessageTable, message).
		Returning(m.message.Columns()...)

	return ib.Build()
}

func (m *messageQuerier) prepareMsgInsertCTE(msg *model.Message) *sqlbuilder.CTEQueryBuilder {
	ib := m.message.
		WithoutTag(IgnoreTag).
		InsertInto(MessageTable, msg).
		Returning(m.message.Columns()...)

	return sqlbuilder.CTEQuery("msg").As(ib)
}

func (m *messageQuerier) InsertLocation(message *model.Message) (string, []any) {
	lib := locationStructure.
		InsertInto(MessageLocationAttachment).
		Cols(locationStructure.Columns()...).
		Returning(locationStructure.Columns()...)

	sel := lib.Select("id").From("msg")
	sel.SelectMore(
		sel.Var(message.Location.Address),
		sel.Var(message.Location.Latitude),
		sel.Var(message.Location.Longitude),
		sel.Var(message.Location.Name),
	)

	ctes := sqlbuilder.With(
		m.prepareMsgInsertCTE(message),
		sqlbuilder.CTEQuery("loc").As(lib),
	)

	query := locationStructure.
		SelectFrom("msg as m").
		With(ctes).
		Select(
			append(
				addAliasToCols("m", m.message.Columns()...),
				addAliasToCols("l", locationStructure.Columns()...)...
			)...
		).
		JoinWithOption(sqlbuilder.InnerJoin, "loc as l", "l.message_id = m.id")

	return query.Build()
}

func (m *messageQuerier) LocationScanFn(msg *model.Message, location *model.Location) []any {
	return append(
		m.message.Addr(msg),
		locationStructure.Addr(location)...
	)
}

func (m *messageQuerier) InsertContact(message *model.Message) (string, []any) {
	cib := contactStructure.
		InsertInto(MessageContactAttachment).
		Cols(contactStructure.Columns()...).
		Returning(contactStructure.Columns()...)

	sel := cib.Select("id").From("msg")
	sel.SelectMore(
		sel.Var(message.Contact.Name),
		sel.Var(message.Contact.Email),
		sel.Var(message.Contact.PhoneNumber),
		sel.Var(message.Contact.Metadata),
	)

	ctes := sqlbuilder.With(
		m.prepareMsgInsertCTE(message),
		sqlbuilder.CTEQuery("con").As(cib),
	)

	query := contactStructure.
		SelectFrom("msg as m").
		With(ctes).
		Select(
			append(
				addAliasToCols("m", m.message.Columns()...),
				addAliasToCols("c", contactStructure.Columns()...)...
			)...,
		).
		JoinWithOption(
			sqlbuilder.InnerJoin,
			"con as c",
			"c.message_id = m.id",
		)

	return query.Build()
}

func (m *messageQuerier) ContactScanFn(message *model.Message) []any {
	return append(
		m.message.Addr(message),
		contactStructure.Addr(message.Contact)...
	)
}

func (m *messageQuerier) prepareDocumentCTE(doc []*model.MessageDocument) *sqlbuilder.CTEQueryBuilder {
	var (
		file_ids []int = make([]int, len(doc))
		names []string = make([]string, len(doc))
		mime []string = make([]string, len(doc)) 
		size []int = make([]int, len(doc))
	)

	for i, d := range doc {
		file_ids[i] = int(d.FileID)
		names[i] = d.Name
		mime[i] = d.Mime
		size[i] = int(d.Size)
	}

	dib := documentStructure.
		WithoutTag(IgnoreTag).
		InsertInto(MessageDocumentTable).
		Returning(documentStructure.Columns()...)
	
	unnestSel := dib.Select("*")
	unnestSel.From(
		fmt.Sprintf(
			"unnest(msg.id,%s::bigint[],%s::text[],%s::varchar[],%s::bigint) as d(message_id, file_id, name, mime, size)",
			unnestSel.Var(file_ids),
			unnestSel.Var(names),
			unnestSel.Var(mime),
			unnestSel.Var(size),

		),
	).
	JoinWithOption(
		sqlbuilder.InnerJoin,
		"msg",
		"1=1",
	)
	

	return sqlbuilder.CTEQuery("docs").As(dib)	
}

func (m *messageQuerier) prepareImageCTE(img []*model.MessageImage) *sqlbuilder.CTEQueryBuilder {
	var (
		file_ids []int = make([]int, len(img))
		mime []string = make([]string, len(img))
		width []int = make([]int, len(img))
		height []int = make([]int, len(img))
	)

	for i, im := range img {
		file_ids[i] = int(im.FileID)
		mime[i] = im.Mime
		width[i] = int(im.Width)
		height[i] = int(im.Height)
	}

	iib := imageStructure.
		WithoutTag(IgnoreTag).
		InsertInto(MessageImageTable).
		Returning(imageStructure.Columns()...)

	unnestSel := iib.Select("*")
		
	unnestSel.From(
		fmt.Sprintf(
			"unnest(msg.id, %s::int[], %s::varchar, %s::int[], %s::int[]) as i(msg_id, file_id, mime, width, height)",
			unnestSel.Var(file_ids),
			unnestSel.Var(mime),
			unnestSel.Var(width),
			unnestSel.Var(height),
		),
	).
	JoinWithOption(
		sqlbuilder.InnerJoin,
		"msg",
		"1=1",
	)

	return sqlbuilder.CTEQuery("img").As(iib)
}

// func (m *messageQuerier) InsertInteractive(message *model.Message) (string, []any) {
// 	mib := m.message.
// 		WithoutTag(IgnoreTag).
// 		InsertInto(MessageTable, message).
// 		Returning(m.message.Columns()...)

// 	var ctes []*sqlbuilder.CTEQueryBuilder
// 	if len(message.Interactive.Header.Documents) > 0 {
// 		ctes = append(
// 			ctes, m.prepareDocumentCTE(message.Interactive.Header.Documents),
// 		)
// 	}

// 	if len(message.Interactive.Header.Images) > 0 {
// 		ctes = append(
// 			ctes, m.prepareImageCTE(message.Interactive.Header.Images),
// 		)
// 	}

// 	if len(ctes) == 0 {
// 		return mib.Build()
// 	}

// 		ctes = append(
// 			[]*sqlbuilder.CTEQueryBuilder{
// 				sqlbuilder.CTEQuery("msg").As(mib),
// 			},
// 			ctes...
// 		)

// 	query := m.message.
// 		SelectFrom("msg").
// 		Select(
// 			addAliasToCols("m", m.message.Columns()...)...
// 		)

// 	for _, cte := range ctes {
// 		if cte.TableName() == "docs" {
// 			query.SelectMore(
// 				addAliasToCols("d", documentStructure.Columns()...)...
// 			)
// 		}

// 		if cte.TableName() == "img" {
// 			query.SelectMore(
// 				addAliasToCols("i", imageStructure.Columns()...)...
// 			).
// 			JoinWithOption(
// 				sqlbuilder.LeftJoin,
// 				query.LateralAs(
// 					imageStructure.Flavor.
// 					NewSelectBuilder().
// 					Select("jsonb_agg(img) as img_jsonb").
// 					From(
// 						imageStructure.Flavor.NewSelectBuilder().
// 						From("img i").
// 						Select(
// 							buildJsonbObject("i", imageStructure.Columns()),
// 						),
// 					),
// 				),
// 				"true",
// 			)
// 		}
// 	}

// 	return query.Build()
// }

func buildJsonbObject(alias string, fields []string) string {
    pairs := make([]string, 0, len(fields)*2)
    for _, f := range fields {
        pairs = append(pairs, fmt.Sprintf("'%s', %s.%s", f, alias, f))
    }
    return fmt.Sprintf("jsonb_build_object(%s)", strings.Join(pairs, ", "))
}