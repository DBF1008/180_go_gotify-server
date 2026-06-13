package stream

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/gotify/server/v2/auth"
	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/stretchr/testify/assert"
)

// multiUserHandler registers a client whose user id and token are taken from the
// query string. Unlike the shared-counter handlers used elsewhere in the tests,
// it is safe to use from connections that are dialed concurrently.
func multiUserHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		uid, _ := strconv.ParseUint(ctx.Query("uid"), 10, 64)
		auth.RegisterClient(ctx, &model.Client{UserID: uint(uid), Token: ctx.Query("token")})
	}
}

func dialUser(url string, uid uint, token string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("%s?uid=%d&token=%s", url, uid, token), nil)
	return conn, err
}

// drainConn keeps a client connection alive by continuously reading from it
// (which also lets gorilla answer server pings) and returns once it is closed.
func drainConn(conn *websocket.Conn) {
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

// assertBroadcastHealthy verifies that the push pipeline still delivers messages
// to a freshly connected client of the given user.
func assertBroadcastHealthy(t *testing.T, api *API, url string, uid uint) {
	t.Helper()

	conn, err := dialUser(url, uid, fmt.Sprintf("u%d-health", uid))
	if err != nil {
		t.Fatalf("dial health client: %v", err)
	}
	defer conn.Close()

	tc := &testingClient{conn: conn, readMessage: make(chan model.MessageExternal, 64), t: t}
	startReading(tc)

	registered := false
	for i := 0; i < 200; i++ {
		if len(clients(api, uid)) > 0 {
			registered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !registered {
		t.Fatal("health client never registered with the stream API")
	}

	want := &model.MessageExternal{ID: 7, Message: "post-storm"}
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		api.Notify(uid, want)
		select {
		case got := <-tc.readMessage:
			assert.Equal(t, *want, got)
			return
		case <-tick.C:
			// retry the notification
		case <-deadline:
			t.Fatal("broadcast pipeline did not deliver to a fresh client after the storm")
		}
	}
}

// TestConcurrentLifecycleStress hammers the stream API with every connection
// lifecycle transition that happens in production at the same time as a stream of
// broadcasts:
//
//   - connections that disconnect on their own (the server-side NotifyClose path),
//   - whole users being deleted (NotifyDeletedUser),
//   - individual clients being revoked / expired and cleaned up
//     (CollectConnectedClientTokens + NotifyDeletedClient, mirroring the router's
//     periodic cleanup job).
//
// Before the fix this reliably panicked with "send on closed channel" (Notify
// wrote to a channel that a closing connection had just closed) and/or stalled
// delivery (Notify held the read lock while writing). The test passes only if no
// panic/data race occurs and broadcasting still works afterwards. Run with -race.
func TestConcurrentLifecycleStress(t *testing.T) {
	mode.Set(mode.TestDev)
	defer leaktest.CheckTimeout(t, 20*time.Second)()

	server, api := bootTestServer(multiUserHandler())
	defer server.Close()
	defer api.Close()

	url := wsURL(server.URL)

	const (
		users       = 6
		perUser     = 3
		broadcaster = 4
		churner     = 4
		notifyOps   = 3000
		churnOps    = 150
		deleteOps   = 400
	)

	// A pool of long-lived connections that the chaos workers broadcast to,
	// disconnect, delete and expire concurrently.
	var poolConns []*websocket.Conn
	for u := uint(1); u <= users; u++ {
		for c := 0; c < perUser; c++ {
			conn, err := dialUser(url, u, fmt.Sprintf("u%d-pool%d", u, c))
			if err != nil {
				t.Fatalf("dial pool client: %v", err)
			}
			drainConn(conn)
			poolConns = append(poolConns, conn)
		}
	}
	defer func() {
		for _, conn := range poolConns {
			conn.Close()
		}
	}()

	waitForConnectedClients(api, users*perUser)

	var wg sync.WaitGroup
	msg := &model.MessageExternal{ID: 1, Message: "stress"}

	// Broadcasters.
	for w := 0; w < broadcaster; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < notifyOps; i++ {
				api.Notify(uint(i%users)+1, msg)
			}
		}()
	}

	// Disconnect churn (drives the server-side NotifyClose path).
	for w := 0; w < churner; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < churnOps; i++ {
				u := uint((w+i)%users) + 1
				conn, err := dialUser(url, u, fmt.Sprintf("u%d-churn%d-%d", u, w, i))
				if err != nil {
					continue
				}
				conn.Close()
			}
		}(w)
	}

	// Deleted users.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < deleteOps; i++ {
			if err := api.NotifyDeletedUser(uint(i%users) + 1); err != nil {
				t.Errorf("NotifyDeletedUser returned an error: %v", err)
				return
			}
		}
	}()

	// Revoked / expired client cleanup (as performed by the router's ticker).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < deleteOps; i++ {
			// Touch the same read path the cleanup job uses.
			_ = api.CollectConnectedClientTokens()
			for u := uint(1); u <= users; u++ {
				api.NotifyDeletedClient(u, fmt.Sprintf("u%d-pool%d", u, i%perUser))
			}
		}
	}()

	wg.Wait()

	// The push pipeline must still be intact for a brand-new client.
	assertBroadcastHealthy(t, api, url, 999)
}

// TestNotifyDoesNotDeadlockOnWedgedClient pins down the lock/lifecycle contract
// deterministically: a connection whose write is wedged must not be able to block
// a forced logout, and the forced logout must release any broadcast that is parked
// on that connection.
//
// Before the fix, Notify held the read lock while sending, so a full write buffer
// blocked NotifyDeletedUser on the write lock (and the eventual channel close then
// panicked the parked Notify). After the fix, Notify sends without the lock and a
// closing client is observed via the done channel, so both unblock promptly.
func TestNotifyDoesNotDeadlockOnWedgedClient(t *testing.T) {
	mode.Set(mode.TestDev)

	// Wedge every write so the per-client write goroutine cannot drain its
	// channel, forcing the buffer to fill. The hook signals unwedged and returns
	// an error once released so the goroutine stops reading the writeJSON hook
	// before the test restores it.
	release := make(chan struct{})
	unwedged := make(chan struct{}, 4)
	oldWrite := writeJSON
	writeJSON = func(conn *websocket.Conn, v interface{}) error {
		<-release
		unwedged <- struct{}{}
		return errors.New("write released")
	}
	// Registered first so it runs last: after the hook is restored and the wedged
	// goroutine has been freed below.
	defer leaktest.CheckTimeout(t, 10*time.Second)()
	// Free the wedged write goroutine and only restore the global hook once its
	// in-flight write has observed the release, so the restore can't race the
	// goroutine's read of writeJSON.
	defer func() {
		close(release)
		select {
		case <-unwedged:
		case <-time.After(3 * time.Second):
		}
		writeJSON = oldWrite
	}()

	server, api := bootTestServer(multiUserHandler())
	defer server.Close()
	defer api.Close()

	url := wsURL(server.URL)

	conn, err := dialUser(url, 1, "wedged")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	waitForConnectedClients(api, 1)

	msg := &model.MessageExternal{ID: 1, Message: "x"}

	// Saturate the write path: the first message is taken by the wedged write
	// goroutine, the second fills the size-1 buffer, the third parks inside
	// publish.
	notifyReturned := make(chan struct{})
	go func() {
		api.Notify(1, msg)
		api.Notify(1, msg)
		api.Notify(1, msg)
		close(notifyReturned)
	}()

	// Let the broadcaster reach the parked send.
	time.Sleep(200 * time.Millisecond)

	// Forced logout must not be blocked by the wedged connection.
	deleteReturned := make(chan struct{})
	go func() {
		api.NotifyDeletedUser(1)
		close(deleteReturned)
	}()

	select {
	case <-deleteReturned:
		// expected: teardown completed despite the wedged client
	case <-time.After(2 * time.Second):
		t.Fatal("NotifyDeletedUser blocked behind a wedged client (read lock held during send)")
	}

	select {
	case <-notifyReturned:
		// expected: closing the client released the parked broadcast
	case <-time.After(2 * time.Second):
		t.Fatal("Notify stayed blocked after the client was closed")
	}
}
