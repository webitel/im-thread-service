package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type (
	messageHistoryStore struct {
		db Querier
	}
)

func NewMessageHistoryStore(db Querier) *messageHistoryStore {
	return &messageHistoryStore{
		db: db,
	}
}

func (s *messageHistoryStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*dto.HistoryMessage, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	messages := make([]*dto.HistoryMessage, 0)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

func scanMessage(rows pgx.Rows) (*dto.HistoryMessage, error) {
	var (
		msg dto.HistoryMessage

		documentsJSON json.RawMessage
		imagesJSON    json.RawMessage
		metadataJSON  json.RawMessage

		cols     = rows.FieldDescriptions()
		scanArgs = make([]any, len(cols))
	)

	for i, col := range cols {
		switch string(col.Name) {
		case "id":
			scanArgs[i] = &msg.ID
		case "thread_id":
			scanArgs[i] = &msg.ThreadID
		case "sender_id":
			scanArgs[i] = &msg.SenderID
		case "receiver_id":
			scanArgs[i] = &msg.ReceiverID
		case "type":
			scanArgs[i] = &msg.Type
		case "body":
			scanArgs[i] = &msg.Body
		case "metadata":
			scanArgs[i] = &metadataJSON
		case "created_at":
			scanArgs[i] = &msg.CreatedAt
		case "updated_at":
			scanArgs[i] = &msg.UpdatedAt
		case "documents":
			scanArgs[i] = &documentsJSON
		case "images":
			scanArgs[i] = &imagesJSON
		default:
			var discard any
			scanArgs[i] = &discard
		}
	}

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}

	if len(documentsJSON) > 0 {
		if err := json.Unmarshal(documentsJSON, &msg.Documents); err != nil {
			return nil, err
		}
	}

	if len(imagesJSON) > 0 {
		if err := json.Unmarshal(imagesJSON, &msg.Images); err != nil {
			return nil, err
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &msg.Metadata); err != nil {
			return nil, err
		}
	}

	return &msg, nil
}
