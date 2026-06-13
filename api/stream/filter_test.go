package stream

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/stretchr/testify/assert"
)

// createFilteredClient creates a WebSocket client that sends a subscription message
// and waits for the filter to be applied before returning.
func createFilteredClient(t *testing.T, url string, filter *model.MessageFilter) *testingClient {
	client := createClient(t, url)

	sub := struct {
		Subscribe *model.MessageFilter `json:"subscribe"`
	}{Subscribe: filter}
	data, err := json.Marshal(sub)
	assert.Nil(t, err)
	err = client.conn.WriteMessage(1, data) // TextMessage
	assert.Nil(t, err)

	// Start the read loop
	go func() {
		for {
			_, payload, err := client.conn.ReadMessage()
			if err != nil {
				return
			}
			actual := &model.MessageExternal{}
			json.NewDecoder(bytes.NewBuffer(payload)).Decode(actual)
			client.readMessage <- *actual
		}
	}()

	return client
}

// waitForFilter polls until the client's filter is set or times out.
func waitForFilter(api *API, userID uint, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		api.lock.RLock()
		clients := api.clients[userID]
		for _, c := range clients {
			if c.getFilter() != nil {
				api.lock.RUnlock()
				return true
			}
		}
		api.lock.RUnlock()
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestWS_NoSubscription_ReceivesAll(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	user := testClient(t, wsURL)
	defer user.conn.Close()

	waitForConnectedClients(api, 1)

	api.Notify(1, &model.MessageExternal{ID: 1, ApplicationID: 1, Message: "app1 low", Priority: intPtr(1)})
	user.expectMessage(&model.MessageExternal{ID: 1, ApplicationID: 1, Message: "app1 low", Priority: intPtr(1)})

	api.Notify(1, &model.MessageExternal{ID: 2, ApplicationID: 2, Message: "app2 high", Priority: intPtr(8)})
	user.expectMessage(&model.MessageExternal{ID: 2, ApplicationID: 2, Message: "app2 high", Priority: intPtr(8)})

	api.Notify(1, &model.MessageExternal{ID: 3, ApplicationID: 1, Message: "app1 high", Priority: intPtr(10)})
	user.expectMessage(&model.MessageExternal{ID: 3, ApplicationID: 1, Message: "app1 high", Priority: intPtr(10)})
}

func TestWS_FilterByApp(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	user := createFilteredClient(t, wsURL, &model.MessageFilter{ApplicationID: uintPtr(1)})
	defer user.conn.Close()

	waitForConnectedClients(api, 1)
	assert.True(t, waitForFilter(api, 1, 2*time.Second), "filter should be applied")

	api.Notify(1, &model.MessageExternal{ID: 1, ApplicationID: 1, Message: "app1 msg", Priority: intPtr(5)})
	user.expectMessage(&model.MessageExternal{ID: 1, ApplicationID: 1, Message: "app1 msg", Priority: intPtr(5)})

	api.Notify(1, &model.MessageExternal{ID: 2, ApplicationID: 2, Message: "app2 msg", Priority: intPtr(5)})
	user.expectNoMessage()

	api.Notify(1, &model.MessageExternal{ID: 3, ApplicationID: 1, Message: "app1 msg2", Priority: intPtr(3)})
	user.expectMessage(&model.MessageExternal{ID: 3, ApplicationID: 1, Message: "app1 msg2", Priority: intPtr(3)})
}

func TestWS_FilterByPriorityMin(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	user := createFilteredClient(t, wsURL, &model.MessageFilter{PriorityMin: intPtr(5)})
	defer user.conn.Close()

	waitForConnectedClients(api, 1)
	assert.True(t, waitForFilter(api, 1, 2*time.Second), "filter should be applied")

	api.Notify(1, &model.MessageExternal{ID: 1, Message: "low", Priority: intPtr(2)})
	user.expectNoMessage()

	api.Notify(1, &model.MessageExternal{ID: 2, Message: "high", Priority: intPtr(8)})
	user.expectMessage(&model.MessageExternal{ID: 2, Message: "high", Priority: intPtr(8)})

	api.Notify(1, &model.MessageExternal{ID: 3, Message: "medium", Priority: intPtr(5)})
	user.expectMessage(&model.MessageExternal{ID: 3, Message: "medium", Priority: intPtr(5)})
}

func TestWS_CombinedFilter(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	filter := &model.MessageFilter{
		ApplicationID: uintPtr(1),
		PriorityMin:   intPtr(5),
	}
	user := createFilteredClient(t, wsURL, filter)
	defer user.conn.Close()

	waitForConnectedClients(api, 1)
	assert.True(t, waitForFilter(api, 1, 2*time.Second), "filter should be applied")

	// app1 + high priority → HIT
	api.Notify(1, &model.MessageExternal{ID: 1, ApplicationID: 1, Message: "hit", Priority: intPtr(8)})
	user.expectMessage(&model.MessageExternal{ID: 1, ApplicationID: 1, Message: "hit", Priority: intPtr(8)})

	// app1 + low priority → MISS
	api.Notify(1, &model.MessageExternal{ID: 2, ApplicationID: 1, Message: "low prio", Priority: intPtr(2)})
	user.expectNoMessage()

	// app2 + high priority → MISS (wrong app)
	api.Notify(1, &model.MessageExternal{ID: 3, ApplicationID: 2, Message: "wrong app", Priority: intPtr(10)})
	user.expectNoMessage()

	// app1 + exact min priority → HIT
	api.Notify(1, &model.MessageExternal{ID: 4, ApplicationID: 1, Message: "hit2", Priority: intPtr(5)})
	user.expectMessage(&model.MessageExternal{ID: 4, ApplicationID: 1, Message: "hit2", Priority: intPtr(5)})
}

func TestWS_InvalidSubscription_ReceivesAll(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	client := createClient(t, wsURL)
	err := client.conn.WriteMessage(1, []byte("this is not json"))
	assert.Nil(t, err)

	go func() {
		for {
			_, payload, err := client.conn.ReadMessage()
			if err != nil {
				return
			}
			actual := &model.MessageExternal{}
			json.NewDecoder(bytes.NewBuffer(payload)).Decode(actual)
			client.readMessage <- *actual
		}
	}()
	defer client.conn.Close()

	waitForConnectedClients(api, 1)
	time.Sleep(100 * time.Millisecond) // Let the server process the invalid message

	api.Notify(1, &model.MessageExternal{ID: 1, Message: "msg1", Priority: intPtr(1)})
	client.expectMessage(&model.MessageExternal{ID: 1, Message: "msg1", Priority: intPtr(1)})

	api.Notify(1, &model.MessageExternal{ID: 2, Message: "msg2", Priority: intPtr(10)})
	client.expectMessage(&model.MessageExternal{ID: 2, Message: "msg2", Priority: intPtr(10)})
}

func TestWS_EmptySubscribe_ReceivesAll(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)
	user := createFilteredClient(t, wsURL, &model.MessageFilter{})
	defer user.conn.Close()

	waitForConnectedClients(api, 1)
	assert.True(t, waitForFilter(api, 1, 2*time.Second), "filter should be applied")

	api.Notify(1, &model.MessageExternal{ID: 1, ApplicationID: 1, Message: "any", Priority: intPtr(1)})
	user.expectMessage(&model.MessageExternal{ID: 1, ApplicationID: 1, Message: "any", Priority: intPtr(1)})

	api.Notify(1, &model.MessageExternal{ID: 2, ApplicationID: 2, Message: "any2", Priority: intPtr(10)})
	user.expectMessage(&model.MessageExternal{ID: 2, ApplicationID: 2, Message: "any2", Priority: intPtr(10)})
}

func TestWS_FilteredAndUnfilteredCoexist(t *testing.T) {
	mode.Set(mode.TestDev)
	server, api := bootTestServer(staticUserID())
	defer server.Close()
	defer api.Close()

	wsURL := wsURL(server.URL)

	// One client without filter
	unfiltered := testClient(t, wsURL)
	defer unfiltered.conn.Close()

	// One client with priority filter
	filtered := createFilteredClient(t, wsURL, &model.MessageFilter{PriorityMin: intPtr(5)})
	defer filtered.conn.Close()

	waitForConnectedClients(api, 2)
	assert.True(t, waitForFilter(api, 1, 2*time.Second), "filter should be applied")

	// Low priority message
	api.Notify(1, &model.MessageExternal{ID: 1, Message: "low", Priority: intPtr(1)})
	unfiltered.expectMessage(&model.MessageExternal{ID: 1, Message: "low", Priority: intPtr(1)})
	filtered.expectNoMessage() // filtered out

	// High priority message
	api.Notify(1, &model.MessageExternal{ID: 2, Message: "high", Priority: intPtr(8)})
	unfiltered.expectMessage(&model.MessageExternal{ID: 2, Message: "high", Priority: intPtr(8)})
	filtered.expectMessage(&model.MessageExternal{ID: 2, Message: "high", Priority: intPtr(8)})
}

func uintPtr(v uint) *uint { return &v }
func intPtr(v int) *int    { return &v }
