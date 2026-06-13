package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/test"
	"github.com/gotify/server/v2/test/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestMessageFilterSuite(t *testing.T) {
	suite.Run(t, new(MessageFilterSuite))
}

type MessageFilterSuite struct {
	suite.Suite
	db       *testdb.Database
	a        *MessageAPI
	ctx      *gin.Context
	recorder *httptest.ResponseRecorder
}

func (s *MessageFilterSuite) BeforeTest(suiteName, testName string) {
	mode.Set(mode.TestDev)
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	s.ctx.Request = httptest.NewRequest("GET", "/irrelevant", nil)
	s.db = testdb.NewDB(s.T())
	s.a = &MessageAPI{DB: s.db, Notifier: &noopNotifier{}}
}

func (s *MessageFilterSuite) AfterTest(string, string) {
	s.db.Close()
}

type noopNotifier struct{}

func (n *noopNotifier) Notify(userID uint, message *model.MessageExternal) {}

func (s *MessageFilterSuite) withURL(scheme, host, path, query string) {
	s.ctx.Request.URL = &url.URL{Path: path, RawQuery: query}
	s.ctx.Set("location", &url.URL{Scheme: scheme, Host: host})
}

func (s *MessageFilterSuite) parsePagedMessages() *model.PagedMessages {
	s.T().Helper()
	var result model.PagedMessages
	test.BodyEquals(s.T(), &result, s.recorder)
	// Re-read to get the struct since BodyEquals consumes the body
	return nil
}

func (s *MessageFilterSuite) getMessages(userID uint, query string) *httptest.ResponseRecorder {
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	s.ctx.Request = httptest.NewRequest("GET", "/message?"+query, nil)
	s.ctx.Request.URL = &url.URL{Path: "/message", RawQuery: query}
	s.ctx.Set("location", &url.URL{Scheme: "http", Host: "example.com"})
	test.WithUser(s.ctx, userID)
	s.a.GetMessages(s.ctx)
	return s.recorder
}

func (s *MessageFilterSuite) getAppMessages(userID uint, appID string, query string) *httptest.ResponseRecorder {
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	s.ctx.Request = httptest.NewRequest("GET", "/application/"+appID+"/message?"+query, nil)
	s.ctx.Request.URL = &url.URL{Path: "/application/" + appID + "/message", RawQuery: query}
	s.ctx.Set("location", &url.URL{Scheme: "http", Host: "example.com"})
	s.ctx.Params = gin.Params{{Key: "id", Value: appID}}
	test.WithUser(s.ctx, userID)
	s.a.GetMessagesWithApplication(s.ctx)
	return s.recorder
}

// TestNoFilter_BackwardCompat: No filter params should return all messages (backward compatible).
func (s *MessageFilterSuite) TestNoFilter_BackwardCompat() {
	user := s.db.User(5)
	first := user.App(1).NewMessage(1)
	second := user.App(2).NewMessage(2)
	firstExternal := toExternalMessage(&first)
	secondExternal := toExternalMessage(&second)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "")
	s.a.GetMessages(s.ctx)

	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{secondExternal, firstExternal},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
	assert.Equal(s.T(), 200, s.recorder.Code)
}

// TestFilterByApplicationID: Filter by appid should return only that app's messages.
func (s *MessageFilterSuite) TestFilterByApplicationID() {
	user := s.db.User(5)
	app1Msg1 := user.App(1).NewMessage(1)
	user.App(2).NewMessage(2)
	app1Msg3 := user.App(1).NewMessage(3)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "appid=1")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	// Verify response contains only app 1 messages, not app 2
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&app1Msg3), toExternalMessage(&app1Msg1)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestFilterByPriorityExact: Filter by exact priority value.
func (s *MessageFilterSuite) TestFilterByPriorityExact() {
	user := s.db.User(5)
	user.App(1).NewMessageWithPriority(1, 0)
	user.App(1).NewMessageWithPriority(2, 3)
	target := user.App(1).NewMessageWithPriority(3, 5)
	user.App(1).NewMessageWithPriority(4, 7)
	user.App(1).NewMessageWithPriority(5, 10)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority=5")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	body := s.recorder.Body.String()
	assert.Contains(s.T(), body, `"priority":5`)
	// Should only return the one message with priority 5
	targetExternal := toExternalMessage(&target)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 1, Next: ""},
		Messages: []*model.MessageExternal{targetExternal},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestFilterByPriorityZero: Priority=0 is a valid filter value.
func (s *MessageFilterSuite) TestFilterByPriorityZero() {
	user := s.db.User(5)
	zeroMsg := user.App(1).NewMessageWithPriority(1, 0)
	user.App(1).NewMessageWithPriority(2, 5)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority=0")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	zeroExternal := toExternalMessage(&zeroMsg)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 1, Next: ""},
		Messages: []*model.MessageExternal{zeroExternal},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestFilterByPriorityRange: Filter by priority_min and priority_max.
func (s *MessageFilterSuite) TestFilterByPriorityRange() {
	user := s.db.User(5)
	user.App(1).NewMessageWithPriority(1, 0)
	msg3 := user.App(1).NewMessageWithPriority(2, 3)
	msg5 := user.App(1).NewMessageWithPriority(3, 5)
	msg7 := user.App(1).NewMessageWithPriority(4, 7)
	user.App(1).NewMessageWithPriority(5, 10)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority_min=3&priority_max=7")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 3, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&msg7), toExternalMessage(&msg5), toExternalMessage(&msg3)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestFilterByTimeWindow: Filter by before and after timestamps.
func (s *MessageFilterSuite) TestFilterByTimeWindow() {
	user := s.db.User(5)
	baseTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	oldMsg := user.App(1).NewMessageFull(1, 5, "old", baseTime.Add(-2*time.Hour))
	_ = oldMsg
	inWindow1 := user.App(1).NewMessageFull(2, 5, "in1", baseTime.Add(-30*time.Minute))
	inWindow2 := user.App(1).NewMessageFull(3, 5, "in2", baseTime.Add(30*time.Minute))
	newMsg := user.App(1).NewMessageFull(4, 5, "new", baseTime.Add(2*time.Hour))
	_ = newMsg

	after := baseTime.Add(-1 * time.Hour)
	before := baseTime.Add(1 * time.Hour)
	query := "after=" + after.Format(time.RFC3339) + "&before=" + before.Format(time.RFC3339)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", query)
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&inWindow2), toExternalMessage(&inWindow1)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestCombinedFilters: Multiple filter conditions intersect.
func (s *MessageFilterSuite) TestCombinedFilters() {
	user := s.db.User(5)
	baseTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// app1, high priority, recent → HIT
	hit := user.App(1).NewMessageFull(1, 8, "hit", baseTime)
	// app1, low priority, recent → MISS (priority too low)
	user.App(1).NewMessageFull(2, 2, "miss_prio", baseTime)
	// app2, high priority, recent → MISS (wrong app)
	user.App(2).NewMessageFull(3, 9, "miss_app", baseTime)
	// app1, high priority, old → MISS (too old)
	user.App(1).NewMessageFull(4, 7, "miss_time", baseTime.Add(-3*time.Hour))

	after := baseTime.Add(-1 * time.Hour)
	query := "appid=1&priority_min=5&after=" + after.Format(time.RFC3339)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", query)
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 1, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&hit)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestPaginationPreservesFilters: Next-page URL contains all active filter params.
func (s *MessageFilterSuite) TestPaginationPreservesFilters() {
	user := s.db.User(5)
	app := user.App(1)
	// Create 10 high-priority messages
	for i := uint(10); i >= 1; i-- {
		app.NewMessageWithPriority(i, 8)
	}

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "appid=1&priority_min=5&limit=3")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	body := s.recorder.Body.String()

	// Next URL should contain appid, priority_min, limit, and since
	assert.Contains(s.T(), body, `"next"`)
	assert.Contains(s.T(), body, `appid=1`)
	assert.Contains(s.T(), body, `priority_min=5`)
	assert.Contains(s.T(), body, `limit=3`)
	assert.Contains(s.T(), body, `since=`)
}

// TestPaginationContinuity: Paging through filtered results has no gaps or duplicates.
func (s *MessageFilterSuite) TestPaginationContinuity() {
	user := s.db.User(5)
	app := user.App(1)
	// Create 15 messages with priority=5
	for i := uint(15); i >= 1; i-- {
		app.NewMessageWithPriority(i, 5)
	}

	var allIDs []uint
	since := uint(0)

	for page := 0; page < 5; page++ {
		query := "priority=5&limit=3"
		if since > 0 {
			query += "&since=" + strings.TrimRight(strings.TrimRight(
				strings.Replace(
					strings.Replace(
						string(rune(since+'0')), string(rune(since+'0')), string(rune(since+'0')), 1,
					), "", "", 1,
				), "0"), "")
		}

		s.recorder = httptest.NewRecorder()
		s.ctx, _ = gin.CreateTestContext(s.recorder)
		rawQuery := "priority=5&limit=3"
		if since > 0 {
			rawQuery += "&since=" + uintToString(since)
		}
		s.ctx.Request = httptest.NewRequest("GET", "/message?"+rawQuery, nil)
		s.ctx.Request.URL = &url.URL{Path: "/message", RawQuery: rawQuery}
		s.ctx.Set("location", &url.URL{Scheme: "http", Host: "example.com"})
		test.WithUser(s.ctx, 5)
		s.a.GetMessages(s.ctx)

		assert.Equal(s.T(), 200, s.recorder.Code)

		// Parse the response manually
		var result struct {
			Paging struct {
				Next  string `json:"next"`
				Since uint   `json:"since"`
				Size  int    `json:"size"`
			} `json:"paging"`
			Messages []struct {
				ID uint `json:"id"`
			} `json:"messages"`
		}

		body := s.recorder.Body.String()
		decoder := json.NewDecoder(strings.NewReader(body))
		err := decoder.Decode(&result)
		assert.NoError(s.T(), err)

		for _, m := range result.Messages {
			allIDs = append(allIDs, m.ID)
		}

		if result.Paging.Since == 0 {
			break
		}
		since = result.Paging.Since
	}

	// Should have all 15 IDs, no duplicates, all unique
	assert.Len(s.T(), allIDs, 15)
	seen := make(map[uint]bool)
	for _, id := range allIDs {
		assert.False(s.T(), seen[id], "duplicate ID %d in pagination", id)
		seen[id] = true
	}

	// IDs should be in descending order (newest first)
	for i := 1; i < len(allIDs); i++ {
		assert.True(s.T(), allIDs[i] < allIDs[i-1], "IDs not in descending order: %v", allIDs)
	}
}

// TestEmptyResult: Restrictive filter returns empty array.
func (s *MessageFilterSuite) TestEmptyResult() {
	user := s.db.User(5)
	user.App(1).NewMessageWithPriority(1, 1)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority=99")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 0, Next: ""},
		Messages: []*model.MessageExternal{},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestAppEndpointWithExtraFilter: /application/:id/message with additional priority filter.
func (s *MessageFilterSuite) TestAppEndpointWithExtraFilter() {
	user := s.db.User(5)
	app := user.App(2)
	app.NewMessageWithPriority(1, 1)
	highMsg := app.NewMessageWithPriority(2, 8)
	app.NewMessageWithPriority(3, 3)

	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.withURL("http", "example.com", "/application/2/message", "priority_min=5")
	s.a.GetMessagesWithApplication(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 1, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&highMsg)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestInvalidLimit_Still400: Invalid limit should still return 400.
func (s *MessageFilterSuite) TestInvalidLimit_Still400() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "limit=555&priority=5")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

// TestCrossUserIsolation: Filter results don't leak across users.
func (s *MessageFilterSuite) TestCrossUserIsolation() {
	user1 := s.db.User(4)
	user1.App(1).NewMessageWithPriority(1, 10)

	user2 := s.db.User(5)
	user2.App(2).NewMessageWithPriority(2, 10)

	// Query as user 4 should only get user 4's message
	test.WithUser(s.ctx, 4)
	s.withURL("http", "example.com", "/message", "priority_min=5")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	body := s.recorder.Body.String()
	assert.Contains(s.T(), body, `"appid":1`)
	assert.NotContains(s.T(), body, `"appid":2`)
}

// TestFilterOnlyMinPriority: priority_min alone works.
func (s *MessageFilterSuite) TestFilterOnlyMinPriority() {
	user := s.db.User(5)
	user.App(1).NewMessageWithPriority(1, 1)
	msg5 := user.App(1).NewMessageWithPriority(2, 5)
	msg8 := user.App(1).NewMessageWithPriority(3, 8)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority_min=5")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&msg8), toExternalMessage(&msg5)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestFilterOnlyMaxPriority: priority_max alone works.
func (s *MessageFilterSuite) TestFilterOnlyMaxPriority() {
	user := s.db.User(5)
	msg1 := user.App(1).NewMessageWithPriority(1, 1)
	msg3 := user.App(1).NewMessageWithPriority(2, 3)
	user.App(1).NewMessageWithPriority(3, 8)

	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/message", "priority_max=3")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{toExternalMessage(&msg3), toExternalMessage(&msg1)},
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

// TestPaginationContinuity_WithFilter: Full page-through of filtered results.
func (s *MessageFilterSuite) TestPaginationContinuity_WithFilter() {
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	// Create 20 messages: 10 from app1 (priority=5), 10 from app2 (priority=10)
	for i := uint(1); i <= 10; i++ {
		app1.NewMessageWithPriority(i, 5)
		app2.NewMessageWithPriority(i+100, 10)
	}

	// Filter: only app1 messages, 3 per page
	var allIDs []uint
	since := uint(0)
	for page := 0; page < 10; page++ {
		s.recorder = httptest.NewRecorder()
		s.ctx, _ = gin.CreateTestContext(s.recorder)
		rawQuery := "appid=1&limit=3"
		if since > 0 {
			rawQuery += "&since=" + uintToString(since)
		}
		s.ctx.Request = httptest.NewRequest("GET", "/message?"+rawQuery, nil)
		s.ctx.Request.URL = &url.URL{Path: "/message", RawQuery: rawQuery}
		s.ctx.Set("location", &url.URL{Scheme: "http", Host: "example.com"})
		test.WithUser(s.ctx, 5)
		s.a.GetMessages(s.ctx)

		assert.Equal(s.T(), 200, s.recorder.Code)

		var result struct {
			Paging struct {
				Since uint `json:"since"`
				Size  int  `json:"size"`
			} `json:"paging"`
			Messages []struct {
				ID    uint `json:"id"`
				AppID uint `json:"appid"`
			} `json:"messages"`
		}
		decoder := json.NewDecoder(strings.NewReader(s.recorder.Body.String()))
		err := decoder.Decode(&result)
		assert.NoError(s.T(), err)

		if result.Paging.Size == 0 {
			break
		}

		for _, m := range result.Messages {
			assert.Equal(s.T(), uint(1), m.AppID, "all messages should be from app 1")
			allIDs = append(allIDs, m.ID)
		}

		if result.Paging.Since == 0 {
			break
		}
		since = result.Paging.Since
	}

	// Should have exactly 10 app1 messages
	assert.Len(s.T(), allIDs, 10)
	// No duplicates
	seen := make(map[uint]bool)
	for _, id := range allIDs {
		assert.False(s.T(), seen[id], "duplicate ID %d", id)
		seen[id] = true
	}
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
