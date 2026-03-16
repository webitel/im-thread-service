package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/webitel/webitel-go-kit/pkg/errors"
)

//#region Buttons

/*
* Questions
* 1. What if user clicked button two times due to bad internet connection, do we need SendButtons idempotency?
* 2. What type of buttons we need right now?
* 3. Can buttons be enabled / disabled or something like this?
* 4. Collect static user location one time or collect dynamic during user session?
* 5. Is user locations share critical for analytics?
* 6. Additional field for message table postback.
* 7. Reply to header for idempotency (see TG realization).
 */

type ButtonActionType string

const (
	URLAction      ButtonActionType = "url"      // On-click open URL
	ReplyAction    ButtonActionType = "reply"    // Send 'quick' reply back
	PostbackAction ButtonActionType = "postback" // Service call
	LocationAction ButtonActionType = "location" // Send user location
	ContactAction  ButtonActionType = "contact"  // Share user contact information
)

const (
	MaxButtonsRowSize int = 3
)

type MessageButtons struct {
	Code         string           `json:"code"`
	Action       ButtonActionType `json:"action"`
	Title        string           `json:"title"`
	CallbackData string           `json:"callback_data"`
	URL          string           `json:"url"`
}

func (ms *MessageButtons) Validate() error {
	if ms.Title == "" && !utf8.ValidString(ms.Title) {
		return errors.InvalidArgument("button title is required and must be correct utf-8 sequence")
	}

	switch ms.Action {
	case URLAction:
		if ms.URL == "" {
			return errors.InvalidArgument("URL is required for URL action button")
		}
	case PostbackAction, ReplyAction:
		if ms.CallbackData == "" {
			return errors.InvalidArgument("callback_data is required for interactive action")
		}
	case LocationAction, ContactAction:
	default:
		return errors.InvalidArgument("unknown button action type", errors.WithValue("action", ms.Action))
	}

	return nil
}

type MessageButtonRow []*MessageButtons

func (mbr *MessageButtonRow) Validate() error {
	if mbr == nil {
		return errors.InvalidArgument("message button row is null")
	}

	if len(*mbr) > MaxButtonsRowSize {
		return errors.InvalidArgument(
			fmt.Sprintf("buttons row has more columns than allowed number: %d", MaxButtonsRowSize),
		)
	}

	for i, b := range *mbr {
		if err := b.Validate(); err != nil {
			return errors.Wrap(
				err,
				errors.WithValue("column index", i),
			)
		}
	}

	return nil
}

type MessageButtonsMatrix []MessageButtonRow

func (mbm *MessageButtonsMatrix) Validate() error {
	if mbm == nil {
		return nil
	}

	if len(*mbm) > MaxButtonsRowSize {
		return errors.InvalidArgument(
			fmt.Sprintf("buttons matrix has more rows than allowed number: %d", MaxButtonsRowSize),
		)
	}

	for i, row := range *mbm {
		if err := row.Validate(); err != nil {
			return errors.Wrap(
				err,
				errors.WithValue("row index", i),
			)
		}
	}

	return nil
}

func (mbm *MessageButtonsMatrix) Value() (driver.Value, error) {
	if mbm == nil {
		return nil, nil
	}

	bytes, err := json.Marshal(mbm)
	if err != nil {
		return nil, err 
	}

	return string(bytes), nil
}

func (mbm *MessageButtonsMatrix) Scan(value any) error {
    if value == nil {
        *mbm = MessageButtonsMatrix{}
        return nil
    }

    bytes, ok := value.([]byte)
    if !ok {
        return fmt.Errorf("failed to unmarshal JSONB value: %T", value)
    }

    return json.Unmarshal(bytes, mbm)
}
//#endregion

