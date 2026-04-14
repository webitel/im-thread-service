package model

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type FlushVariablesCommand struct {
	ThreadID uuid.UUID
	Member   uuid.UUID
	Keys     []string
}

type SetThreadVariablesCommand struct {
	Member    uuid.UUID
	Variables *ThreadVariables
}

type GetThreadVariablesQuery struct {
	Pagination

	Fields    []string
	ThreadIDs uuid.UUIDs
}

type VariableEntry struct {
	Value map[string]any `json:"value"`
	SetBy uuid.UUID      `json:"set_by"`
	SetAt time.Time      `json:"set_at"`
}

type ThreadVariables struct {
	ThreadID      uuid.UUID                `json:"thread_id" db:"thread_id" fieldtag:"default"`
	Variables     map[string]VariableEntry `json:"variables" db:"variables" fieldtag:"default"`
	ThreadMembers []uuid.UUID              `json:"thread_members" db:"thread_members"`

	lock sync.RWMutex `json:"-" db:"-"`
}

func (tv *ThreadVariables) SetVariable(key string, user uuid.UUID, value map[string]any) error {
	tv.lock.Lock()
	defer tv.lock.Unlock()

	if tv.Variables == nil {
		tv.Variables = make(map[string]VariableEntry)
	}

	if existing, ok := tv.Variables[key]; ok {
		if existing.SetBy != user {
			return errors.Forbidden(
				"variable already set by another user",
				errors.WithID("model.thread_variables.set_variable"),
			)
		}
	}

	tv.Variables[key] = VariableEntry{
		Value: value,
		SetBy: user,
		SetAt: time.Now(),
	}

	return nil
}

func (tv *ThreadVariables) GetVariable(key string) (VariableEntry, bool) {
	tv.lock.RLock()
	defer tv.lock.RUnlock()

	val, ok := tv.Variables[key]
	return val, ok
}

func (tv *ThreadVariables) DeleteVariable(key string, user uuid.UUID) error {
	tv.lock.Lock()
	defer tv.lock.Unlock()

	if _, ok := tv.Variables[key]; !ok {
		return errors.NotFound(
			"variable not found",
			errors.WithID("model.thread_variables.delete_variable"),
		)
	}

	if tv.Variables[key].SetBy != user {
		return errors.Forbidden(
			"variable not set by user",
			errors.WithID("model.thread_variables.delete_variable"),
		)
	}

	delete(tv.Variables, key)

	return nil
}

func (tv *ThreadVariables) PrepareSetTopic(setBy uuid.UUID) string {
	return fmt.Sprintf("im_thread.%s.variables.set.%s", tv.ThreadID.String(), setBy.String())
}

func (tv *ThreadVariables) PrepareFlushTopic(flushBy uuid.UUID) string {
	return fmt.Sprintf("im_thread.%s.variables.flush.%s", tv.ThreadID.String(), flushBy.String())
}

func (tv *ThreadVariables) ToPayload() ([]byte, error) {
	if tv == nil {
		return nil, errors.InvalidArgument(
			"thread varaibles object is nil",
			errors.WithID("model.thread_variables.to_payload"),
		)
	}

	payloadStruct := struct {
		Variables map[string]VariableEntry `json:"variables"`
		Members   []uuid.UUID              `json:"members"`
	}{
		Variables: tv.Variables,
		Members:   tv.ThreadMembers,
	}

	payload, err := json.Marshal(payloadStruct)
	if err != nil {
		return nil, errors.Internal(
			"preparing variables payload",
			errors.WithCause(err),
			errors.WithID("model.thread_variables.to_payload"),
		)
	}
	return payload, nil
}
