package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

const botStoppedSystemType = "bot_stopped"

func (c *CommandService) canStopBot(req commandRequest) bool {
	if req.Thread == nil || req.Thread.BotControllerID == nil {
		return false
	}

	if req.Sender != nil && (req.Sender.IsBot || req.Sender.ID == *req.Thread.BotControllerID) {
		return false
	}

	return true
}

func (c *CommandService) handleBotStopCommand(ctx context.Context, req commandRequest) (*model.Message, error) {
	in, t := req.Message, req.Thread

	log := c.logger.With("operation", "command.close", slog.String("thread_id", t.ID.String()))

	var initiatorMemberID uuid.UUID
	if req.Sender != nil {
		initiatorMemberID = req.Sender.ID
	}

	if err := c.bots.ReleaseBotControl(ctx, &dto.ReleaseBotControlRequest{
		ThreadID:          t.ID,
		InitiatorMemberID: initiatorMemberID,
		DomainID:          int(in.DomainID),
	}); err != nil {
		log.ErrorContext(ctx, "failed to release bot control on /close", "err", err)

		return nil, err
	}

	log.InfoContext(ctx, "bot control released via /close")

	return c.buildBotStoppedMessage(in, t), nil
}

func (c *CommandService) buildBotStoppedMessage(in *dto.SendTextRequest, t *model.Thread) *model.Message {
	to := make([]*model.ThreadDialog, 0, len(t.Members))
	for _, m := range t.Members {
		if m != nil && !m.IsBot {
			to = append(to, m)
		}
	}

	msg := &model.Message{
		ThreadID:       t.ID,
		DomainID:       int32(in.DomainID),
		From:           in.From,
		SendTo:         in.To,
		SendAs:         in.SendAs,
		Body:           in.Body,
		To:             to,
		Type:           model.MessageTypeSystem,
		IdempotencyKey: in.SendID,
		Metadata:       model.BuildMetadata(in.Body),
		System: &model.MessageSystem{
			Type:     botStoppedSystemType,
			Metadata: make(map[string]any),
		},
	}

	msg.SetMemberFromSlice(t.Members)

	return msg
}
