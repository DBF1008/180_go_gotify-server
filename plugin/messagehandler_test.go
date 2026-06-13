package plugin

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/plugin/compat"
	"github.com/stretchr/testify/assert"
)

// stubMessageDatabase implements the Database interface but only CreateMessage
// is exercised by the message delivery loop. CreateMessage can be configured to
// fail, and on success it assigns an incrementing ID like the real
// auto-increment column, so tests can assert on persisted ordering and on the
// IDs that get broadcast.
type stubMessageDatabase struct {
	Database // embedded so the struct satisfies Database; unused methods panic if called

	mu        sync.Mutex
	createErr error
	nextID    uint
	created   []*model.Message
}

func (s *stubMessageDatabase) CreateMessage(message *model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
	}
	s.nextID++
	message.ID = s.nextID
	stored := *message // copy so later caller mutations cannot change the record
	s.created = append(s.created, &stored)
	return nil
}

func (s *stubMessageDatabase) createdMessages() []*model.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*model.Message(nil), s.created...)
}

// notification captures a single Notify call.
type notification struct {
	userID  uint
	message model.MessageExternal
}

// recordingNotifier records every broadcast so tests can assert what (if
// anything) was delivered to clients and in which order.
type recordingNotifier struct {
	notified chan notification
}

func newRecordingNotifier(buffer int) *recordingNotifier {
	return &recordingNotifier{notified: make(chan notification, buffer)}
}

func (r *recordingNotifier) Notify(userID uint, message *model.MessageExternal) {
	r.notified <- notification{userID: userID, message: *message}
}

// newTestManager builds a Manager wired only with the pieces the delivery loop
// needs and starts the worker goroutine, returning a handler bound to it.
func newTestManager(t *testing.T, db Database, notifier Notifier) (*Manager, redirectToChannel) {
	t.Helper()
	m := &Manager{
		messages: make(chan MessageWithUserID),
		db:       db,
		notifier: notifier,
	}
	go m.processMessages()
	t.Cleanup(func() { close(m.messages) })
	return m, redirectToChannel{ApplicationID: 7, UserID: 42, Messages: m.messages}
}

func waitForNotification(t *testing.T, notifier *recordingNotifier) notification {
	t.Helper()
	select {
	case n := <-notifier.notified:
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a broadcast")
		return notification{}
	}
}

// TestSendMessage_PersistFailure_NoBroadcastAndReturnsError covers the write-DB
// failure scenario: the plugin must receive the persistence error, nothing may
// be persisted, and crucially the message must not be broadcast (otherwise it
// would appear delivered in real time yet be missing from the history).
func TestSendMessage_PersistFailure_NoBroadcastAndReturnsError(t *testing.T) {
	errBoom := errors.New("database is down")
	db := &stubMessageDatabase{createErr: errBoom}
	notifier := newRecordingNotifier(1)
	_, handler := newTestManager(t, db, notifier)

	err := handler.SendMessage(compat.Message{Title: "t", Message: "m", Priority: 1})

	// The plugin gets a clear failure signal instead of a silent success.
	assert.ErrorIs(t, err, errBoom)

	// Nothing was persisted.
	assert.Empty(t, db.createdMessages())

	// And nothing was broadcast: no "delivered but later lost" message.
	select {
	case n := <-notifier.notified:
		t.Fatalf("expected no broadcast on persistence failure, got %+v", n)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestSendMessage_Success_PersistsThenBroadcasts covers the happy path: the
// message is persisted, SendMessage reports success, and the broadcast carries
// the database-assigned ID (not the zero value) so the real-time notification
// matches the stored message.
func TestSendMessage_Success_PersistsThenBroadcasts(t *testing.T) {
	db := &stubMessageDatabase{}
	notifier := newRecordingNotifier(4)
	_, handler := newTestManager(t, db, notifier)

	priority := 5
	err := handler.SendMessage(compat.Message{
		Title:    "hello",
		Message:  "world",
		Priority: priority,
		Extras:   map[string]interface{}{"client::display": "x"},
	})
	assert.NoError(t, err)

	created := db.createdMessages()
	if assert.Len(t, created, 1) {
		assert.Equal(t, uint(7), created[0].ApplicationID)
		assert.Equal(t, "hello", created[0].Title)
		assert.Equal(t, "world", created[0].Message)
		assert.Equal(t, priority, created[0].Priority)
		assert.NotZero(t, created[0].ID, "persisted message must get a real ID")
	}

	n := waitForNotification(t, notifier)
	assert.Equal(t, uint(42), n.userID)
	assert.Equal(t, created[0].ID, n.message.ID, "broadcast must carry the persisted ID, not 0")
	assert.NotZero(t, n.message.ID)
	assert.Equal(t, uint(7), n.message.ApplicationID)
	assert.Equal(t, "hello", n.message.Title)
	assert.Equal(t, "world", n.message.Message)
	if assert.NotNil(t, n.message.Priority) {
		assert.Equal(t, priority, *n.message.Priority)
	}
}

// TestSendMessage_ConsecutiveMessages_PreserveOrderAndIDs covers several
// messages sent back-to-back: every one must be persisted and broadcast, in the
// same order, each broadcast carrying its own monotonically increasing ID. This
// guards against messages being lost, reordered, or broadcast with a wrong ID.
func TestSendMessage_ConsecutiveMessages_PreserveOrderAndIDs(t *testing.T) {
	const count = 5
	db := &stubMessageDatabase{}
	notifier := newRecordingNotifier(count)
	_, handler := newTestManager(t, db, notifier)

	for i := 0; i < count; i++ {
		err := handler.SendMessage(compat.Message{
			Title:    "t",
			Message:  fmt.Sprintf("message-%d", i),
			Priority: 1,
		})
		assert.NoErrorf(t, err, "SendMessage %d", i)
	}

	// All messages persisted, in send order, with increasing IDs.
	created := db.createdMessages()
	if assert.Len(t, created, count) {
		for i := 0; i < count; i++ {
			assert.Equal(t, fmt.Sprintf("message-%d", i), created[i].Message)
			assert.Equal(t, uint(i+1), created[i].ID)
		}
	}

	// All messages broadcast, in the same order, each carrying its persisted ID.
	for i := 0; i < count; i++ {
		n := waitForNotification(t, notifier)
		assert.Equal(t, uint(42), n.userID)
		assert.Equalf(t, fmt.Sprintf("message-%d", i), n.message.Message, "broadcast %d out of order", i)
		assert.Equalf(t, uint(i+1), n.message.ID, "broadcast %d should carry persisted ID", i)
	}
}
