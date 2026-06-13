package plugin

import (
	"time"

	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/plugin/compat"
)

type redirectToChannel struct {
	ApplicationID uint
	UserID        uint
	Messages      chan MessageWithUserID
}

// MessageWithUserID encapsulates a message with a given user ID.
type MessageWithUserID struct {
	Message model.MessageExternal
	UserID  uint
	// result reports the persistence outcome back to the sender of the message.
	// It is set by SendMessage; it is nil for messages that are not awaiting a
	// delivery result (for example notifications forwarded in tests).
	result chan error
}

// SendMessage persists and broadcasts a message through the manager's delivery
// loop. It blocks until the message has been written to the database and
// returns any persistence error, so the plugin receives a clear failure signal
// when the underlying storage rejects the message. The message is broadcast to
// clients only if it was persisted successfully, keeping the real-time
// notification consistent with what is later returned from the message history.
func (c redirectToChannel) SendMessage(msg compat.Message) error {
	result := make(chan error, 1)
	c.Messages <- MessageWithUserID{
		Message: model.MessageExternal{
			ApplicationID: c.ApplicationID,
			Message:       msg.Message,
			Title:         msg.Title,
			Priority:      &msg.Priority,
			Date:          time.Now(),
			Extras:        msg.Extras,
		},
		UserID: c.UserID,
		result: result,
	}
	return <-result
}
