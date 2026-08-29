package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type BotController interface {
	ReleaseBotControl(ctx context.Context, req *dto.ReleaseBotControlRequest) error
}

type commandRequest struct {
	Thread  *model.Thread
	Message *dto.SendTextRequest
	Sender  *model.ThreadDialog
}

func newCommandRequest(thread *model.Thread, in *dto.SendTextRequest) commandRequest {
	req := commandRequest{Thread: thread, Message: in}
	if thread != nil && in != nil {
		req.Sender = memberByContactID(thread.Members, in.From.ID)
	}

	return req
}

type messageCommand struct {
	applies func(req commandRequest) bool
	handle  func(ctx context.Context, req commandRequest) (*model.Message, error)
}

type CommandService struct {
	bots   BotController
	logger *slog.Logger

	commands map[model.Command]messageCommand
}

func NewCommandService(bots BotController, logger *slog.Logger) *CommandService {
	c := &CommandService{
		bots:   bots,
		logger: logger,
	}

	c.commands = c.buildCommands()

	return c
}

func (c *CommandService) buildCommands() map[model.Command]messageCommand {
	return map[model.Command]messageCommand{
		model.CommandClose: {
			applies: c.canStopBot,
			handle:  c.handleBotStopCommand,
		},
	}
}

func (c *CommandService) Dispatch(ctx context.Context, thread *model.Thread, in *dto.SendTextRequest) (*model.Message, bool, error) {
	req := newCommandRequest(thread, in)

	cmd, ok := c.lookupCommand(req)
	if !ok {
		return nil, false, nil
	}

	msg, err := cmd.handle(ctx, req)

	return msg, true, err
}

func (c *CommandService) lookupCommand(req commandRequest) (messageCommand, bool) {
	cmd, ok := c.commands[model.Command(strings.TrimSpace(req.Message.Body))]
	if !ok {
		return messageCommand{}, false
	}

	if cmd.applies != nil && !cmd.applies(req) {
		return messageCommand{}, false
	}

	return cmd, true
}
