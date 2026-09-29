package pubsub

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
)

// fakePublisher records published topics and can fail on a specific topic.
type fakePublisher struct {
	topics []string
	failOn map[string]error
}

func (p *fakePublisher) Publish(topic string, _ ...*message.Message) error {
	p.topics = append(p.topics, topic)
	if p.failOn != nil {
		if err := p.failOn[topic]; err != nil {
			return err
		}
	}

	return nil
}

func (p *fakePublisher) Close() error { return nil }

func testMsg(eventType string, payload []byte) *message.Message {
	m := message.NewMessage("uuid-1", payload)
	m.Metadata.Set("event_type", eventType)

	return m
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHandleOutboxEvent_PublishesToRoutingKey(t *testing.T) {
	pub := &fakePublisher{}
	msg := testMsg(event.MessageCreatedEvent, []byte(`{}`))
	msg.Metadata.Set(MetadataRoutingKey, "im_message.1.message.created.v1")

	require.NoError(t, handleOutboxEvent(msg, pub, discardLog()))
	assert.Equal(t, []string{"im_message.1.message.created.v1"}, pub.topics)
}

func TestHandleOutboxEvent_FallbackTopic(t *testing.T) {
	pub := &fakePublisher{}

	require.NoError(t, handleOutboxEvent(testMsg(event.MessageCreatedEvent, []byte(`{}`)), pub, discardLog()))
	assert.Equal(t, []string{DefaultFallbackTopic}, pub.topics)
}

// A publish error must surface so the retry middleware re-delivers it.
func TestHandleOutboxEvent_PublishError_Retries(t *testing.T) {
	boom := errors.New("broker down")
	pub := &fakePublisher{failOn: map[string]error{DefaultFallbackTopic: boom}}

	err := handleOutboxEvent(testMsg(event.MessageCreatedEvent, []byte(`{}`)), pub, discardLog())
	require.ErrorIs(t, err, boom)
}

// Cleanup must key on the subscriber's real consumer group, or it never frees rows.
func TestOutboxCleanupOptions_UseSubscriberConsumerGroup(t *testing.T) {
	assert.Equal(t, []string{ConsumerGroupName}, outboxCleanupOptions().ConsumerGroups)
	assert.Equal(t, "im-thread-outbox-forwarder", ConsumerGroupName, "must match messages_offsets.consumer_group")
}
