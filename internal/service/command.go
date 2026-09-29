package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

const commandPrefix = "/"

type Command interface {
	Name() model.Command
	CanExecute(req CommandRequest) bool
	Execute(ctx context.Context, req CommandRequest) (*model.Message, error)
}

type BotController interface {
	ReleaseBotControl(ctx context.Context, req *dto.ReleaseBotControlRequest) error
}

type CommandRequest struct {
	Thread  *model.Thread
	Message *dto.SendTextRequest
	Sender  *model.ThreadDialog
}

func newCommandRequest(thread *model.Thread, in *dto.SendTextRequest) CommandRequest {
	req := CommandRequest{Thread: thread, Message: in}
	if thread != nil {
		req.Sender = memberByContactID(thread.Members, in.From.ID)
	}

	return req
}

func memberByContactID(members []*model.ThreadDialog, contactID uuid.UUID) *model.ThreadDialog {
	for _, m := range members {
		if m != nil && m.ContactID == contactID {
			return m
		}
	}

	return nil
}

type CommandDispatcher struct {
	commands map[model.Command]Command
}

func NewCommandDispatcher(commands []Command) *CommandDispatcher {
	registry := make(map[model.Command]Command, len(commands))
	for _, cmd := range commands {
		registry[cmd.Name()] = cmd
	}

	return &CommandDispatcher{commands: registry}
}

func (d *CommandDispatcher) Dispatch(ctx context.Context, thread *model.Thread, in *dto.SendTextRequest) (*model.Message, bool, error) {
	if in == nil {
		return nil, false, nil
	}

	cmd, ok := d.lookup(in.Body)
	if !ok {
		return nil, false, nil
	}

	req := newCommandRequest(thread, in)
	if !cmd.CanExecute(req) {
		return nil, false, nil
	}

	msg, err := cmd.Execute(ctx, req)

	return msg, true, err
}

func (d *CommandDispatcher) lookup(body string) (Command, bool) {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, commandPrefix) {
		return nil, false
	}

	cmd, ok := d.commands[model.Command(body)]

	return cmd, ok
}
