package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/auth"
	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/test"
	"github.com/gotify/server/v2/test/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestMessageSuite(t *testing.T) {
	suite.Run(t, new(MessageSuite))
}

type MessageSuite struct {
	suite.Suite
	db              *testdb.Database
	a               *MessageAPI
	ctx             *gin.Context
	recorder        *httptest.ResponseRecorder
	notifiedMessage *model.MessageExternal
}

func (s *MessageSuite) BeforeTest(suiteName, testName string) {
	mode.Set(mode.TestDev)
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	s.ctx.Request = httptest.NewRequest("GET", "/irrelevant", nil)
	s.db = testdb.NewDB(s.T())
	s.notifiedMessage = nil
	s.a = &MessageAPI{DB: s.db, Notifier: s}
}

func (s *MessageSuite) AfterTest(string, string) {
	s.db.Close()
}

func (s *MessageSuite) Notify(userID uint, msg *model.MessageExternal) {
	s.notifiedMessage = msg
}

func (s *MessageSuite) Test_ensureCorrectJsonRepresentation() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")

	actual := &model.PagedMessages{
		Paging: model.Paging{Limit: 5, Since: 122, Size: 5, Next: "http://example.com/message?limit=5&since=122"},
		Messages: []*model.MessageExternal{{ID: 55, ApplicationID: 2, Message: "hi", Title: "hi", Date: t, Priority: intPtr(4), Extras: map[string]interface{}{
			"test::string": "string",
			"test::array":  []interface{}{1, 2, 3},
			"test::int":    1,
			"test::float":  0.5,
		}}},
	}
	test.JSONEquals(s.T(), actual, `{"paging": {"limit":5, "since": 122, "size": 5, "next": "http://example.com/message?limit=5&since=122"},
                                              "messages": [{"id":55,"appid":2,"message":"hi","title":"hi","priority":4,"date":"2017-01-02T00:00:00Z","extras":{"test::string":"string","test::array":[1,2,3],"test::int":1,"test::float":0.5}}]}`)
}

func (s *MessageSuite) Test_GetMessages() {
	user := s.db.User(5)
	first := user.App(1).NewMessage(1)
	second := user.App(2).NewMessage(2)
	firstExternal := toExternalMessage(&first)
	secondExternal := toExternalMessage(&second)

	test.WithUser(s.ctx, 5)
	s.a.GetMessages(s.ctx)

	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 2, Next: ""},
		Messages: []*model.MessageExternal{secondExternal, firstExternal},
	}

	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessages_WithLimit_ReturnsNext() {
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	var messages []*model.Message
	for i := 100; i >= 1; i -= 2 {
		one := app2.NewMessage(uint(i))
		two := app1.NewMessage(uint(i - 1))
		messages = append(messages, &one, &two)
	}

	s.withURL("http", "example.com", "/messages", "limit=5")
	test.WithUser(s.ctx, 5)
	s.a.GetMessages(s.ctx)

	// Since: entries with ids from 100 - 96 will be returned (5 entries)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 5, Size: 5, Since: 96, Next: "http://example.com/messages?limit=5&since=96"},
		Messages: toExternalMessages(messages[:5]),
	}

	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessages_WithLimit_WithSince_ReturnsNext() {
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	var messages []*model.Message
	for i := 100; i >= 1; i -= 2 {
		one := app2.NewMessage(uint(i))
		two := app1.NewMessage(uint(i - 1))
		messages = append(messages, &one, &two)
	}

	s.withURL("http", "example.com", "/messages", "limit=13&since=55")
	test.WithUser(s.ctx, 5)
	s.a.GetMessages(s.ctx)

	// Since: entries with ids from 54 - 42 will be returned (13 entries)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 13, Size: 13, Since: 42, Next: "http://example.com/messages?limit=13&since=42"},
		Messages: toExternalMessages(messages[46 : 46+13]),
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessages_BadRequestOnInvalidLimit() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/messages", "limit=555")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_GetMessages_BadRequestOnInvalidLimit_Negative() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/messages", "limit=-5")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_GetMessagesWithToken_InvalidLimit_BadRequest() {
	s.db.User(4).App(2).NewMessage(1)

	test.WithUser(s.ctx, 4)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.withURL("http", "example.com", "/messages", "limit=555")
	s.a.GetMessagesWithApplication(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_GetMessagesWithToken() {
	msg := s.db.User(4).App(2).NewMessage(1)

	test.WithUser(s.ctx, 4)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.GetMessagesWithApplication(s.ctx)

	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 100, Size: 1, Next: ""},
		Messages: toExternalMessages([]*model.Message{&msg}),
	}

	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessagesWithToken_WithLimit_ReturnsNext() {
	user := s.db.User(5)
	app1 := user.App(2)
	var messages []*model.Message
	for i := 100; i >= 1; i-- {
		msg := app1.NewMessage(uint(i))
		messages = append(messages, &msg)
	}

	s.withURL("http", "example.com", "/app/2/message", "limit=9")
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.GetMessagesWithApplication(s.ctx)

	// Since: entries with ids from 100 - 92 will be returned (9 entries)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 9, Size: 9, Since: 92, Next: "http://example.com/app/2/message?limit=9&since=92"},
		Messages: toExternalMessages(messages[:9]),
	}

	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessagesWithToken_WithLimit_WithSince_ReturnsNext() {
	user := s.db.User(5)
	app1 := user.App(2)
	var messages []*model.Message
	for i := 100; i >= 1; i-- {
		msg := app1.NewMessage(uint(i))
		messages = append(messages, &msg)
	}

	s.withURL("http", "example.com", "/app/2/message", "limit=13&since=55")
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.GetMessagesWithApplication(s.ctx)

	// Since: entries with ids from 54 - 42 will be returned (13 entries)
	expected := &model.PagedMessages{
		Paging:   model.Paging{Limit: 13, Size: 13, Since: 42, Next: "http://example.com/app/2/message?limit=13&since=42"},
		Messages: toExternalMessages(messages[46 : 46+13]),
	}
	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *MessageSuite) Test_GetMessagesWithToken_withWrongUser_expectNotFound() {
	s.db.User(4)
	s.db.User(5).App(2).Message(66)

	test.WithUser(s.ctx, 4)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.GetMessagesWithApplication(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessage_invalidID() {
	s.ctx.Params = gin.Params{{Key: "id", Value: "string"}}

	s.a.DeleteMessage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessage_notExistingID() {
	s.db.User(1).App(5).Message(55)

	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	s.a.DeleteMessage(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessage_existingIDButNotOwner() {
	s.db.User(1).App(10).Message(100)
	s.db.User(2)

	test.WithUser(s.ctx, 2)
	s.ctx.Params = gin.Params{{Key: "id", Value: "100"}}
	s.a.DeleteMessage(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessage() {
	s.db.User(6).App(1).Message(50)

	test.WithUser(s.ctx, 6)
	s.ctx.Params = gin.Params{{Key: "id", Value: "50"}}
	s.a.DeleteMessage(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	s.db.AssertMessageNotExist(50)
}

func (s *MessageSuite) Test_DeleteMessageWithID() {
	s.db.User(2).AppWithToken(5, "mytoken").Message(55)

	test.WithUser(s.ctx, 2)
	s.ctx.Params = gin.Params{{Key: "id", Value: "5"}}
	s.a.DeleteMessageWithApplication(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	s.db.AssertMessageNotExist(55)
}

func (s *MessageSuite) Test_DeleteMessageWithToken_notExistingID() {
	s.db.User(2).AppWithToken(1, "wrong").Message(1)

	test.WithUser(s.ctx, 2)
	s.ctx.Params = gin.Params{{Key: "id", Value: "55"}}
	s.a.DeleteMessageWithApplication(s.ctx)

	s.db.AssertMessageExist(1)
	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessageWithToken_notOwner() {
	s.db.User(4)
	s.db.User(2).App(55).Message(5)

	test.WithUser(s.ctx, 4)
	s.ctx.Params = gin.Params{{Key: "id", Value: "55"}}
	s.a.DeleteMessageWithApplication(s.ctx)

	s.db.AssertMessageExist(5)
	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *MessageSuite) Test_DeleteMessages() {
	userBuilder := s.db.User(4)
	userBuilder.App(5).Message(5).Message(6)
	userBuilder.App(2).Message(7).Message(8)
	s.db.User(5).App(7).Message(22)

	test.WithUser(s.ctx, 4)
	s.a.DeleteMessages(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	s.db.AssertMessageExist(22)
	s.db.AssertMessageNotExist(5, 6, 7, 8)
}

func (s *MessageSuite) Test_CreateMessage_onJson_allParams() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")

	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(7, "app-token"))
	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle", "message": "mymessage", "priority": 1}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(7)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{ID: 1, ApplicationID: 7, Title: "mytitle", Message: "mymessage", Priority: intPtr(1), Date: t}
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), expected, s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_WithDefaultPriority() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")

	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithTokenAndDefaultPriority(8, "app-token", 5))
	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle", "message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{ID: 1, ApplicationID: 8, Title: "mytitle", Message: "mymessage", Priority: intPtr(5), Date: t}
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), expected, s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_WithTitle() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(5, "app-token"))
	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle", "message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(5)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{ID: 1, ApplicationID: 5, Title: "mytitle", Message: "mymessage", Date: t, Priority: intPtr(0)}
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), expected, s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_failWhenNoMessage() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(1, "app-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	if msgs, err := s.db.GetMessagesByApplication(1); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), msgs)
	}
	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Nil(s.T(), s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_WithoutTitle() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithTokenAndName(8, "app-token", "Application name"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), "Application name", msgs[0].Title)
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), "mymessage", s.notifiedMessage.Message)
}

func (s *MessageSuite) Test_CreateMessage_WithBlankTitle() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithTokenAndName(8, "app-token", "Application name"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"message": "mymessage", "title": "  "}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), "Application name", msgs[0].Title)
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), "mymessage", msgs[0].Message)
}

func (s *MessageSuite) Test_CreateMessage_IgnoreID() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithTokenAndName(8, "app-token", "Application name"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"message": "mymessage", "id": 1337}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.NotEqual(s.T(), msgs[0].ID, uint(1337))
	assert.Equal(s.T(), 200, s.recorder.Code)
}

func (s *MessageSuite) Test_CreateMessage_WithExtras() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithTokenAndName(8, "app-token", "Application name"))

	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"message": "mymessage", "title": "msg with extras", "extras": {"gotify::test":{"int":1,"float":0.5,"string":"test","array":[1,2,3]}}}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{
		ID:            1,
		ApplicationID: 8,
		Message:       "mymessage",
		Title:         "msg with extras",
		Date:          t,
		Priority:      intPtr(0),
		Extras: map[string]interface{}{
			"gotify::test": map[string]interface{}{
				"string": "test",
				"array":  []interface{}{float64(1), float64(2), float64(3)},
				"int":    float64(1),
				"float":  float64(0.5),
			},
		},
	}
	assert.Len(s.T(), msgs, 1)

	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))

	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), uint(1), s.notifiedMessage.ID)
}

func (s *MessageSuite) Test_CreateMessage_failWhenPriorityNotNumber() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(8, "app-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle", "message": "mymessage", "priority": "asd"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Nil(s.T(), s.notifiedMessage)
	if msgs, err := s.db.GetMessagesByApplication(1); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), msgs)
	}
}

func (s *MessageSuite) Test_CreateMessage_onQueryData() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(2, "app-token"))

	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	s.ctx.Request = httptest.NewRequest("POST", "/message?title=mytitle&message=mymessage&priority=1", nil)
	s.ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.a.CreateMessage(s.ctx)

	expected := &model.MessageExternal{ID: 1, ApplicationID: 2, Title: "mytitle", Message: "mymessage", Priority: intPtr(1), Date: t}

	msgs, err := s.db.GetMessagesByApplication(2)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), uint(1), s.notifiedMessage.ID)
}

func (s *MessageSuite) Test_CreateMessage_onFormData() {
	auth.RegisterApplication(s.ctx, s.db.User(4).NewAppWithToken(99, "app-token"))

	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`title=mytitle&message=mymessage&priority=1`))
	s.ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.a.CreateMessage(s.ctx)

	expected := &model.MessageExternal{ID: 1, ApplicationID: 99, Title: "mytitle", Message: "mymessage", Priority: intPtr(1), Date: t}
	msgs, err := s.db.GetMessagesByApplication(99)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), uint(1), s.notifiedMessage.ID)
}

func (s *MessageSuite) Test_CreateMessage_clientToken_usesBodyAppId() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	user := s.db.User(4)
	user.NewAppWithToken(7, "app-token")
	auth.RegisterClient(s.ctx, user.NewClientWithToken(1, "client-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"appid": 7, "title": "mytitle", "message": "mymessage", "priority": 1}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(7)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{ID: 1, ApplicationID: 7, Title: "mytitle", Message: "mymessage", Priority: intPtr(1), Date: t}
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), expected, s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_clientToken_missingAppId_400() {
	user := s.db.User(4)
	user.NewAppWithToken(7, "app-token")
	auth.RegisterClient(s.ctx, user.NewClientWithToken(1, "client-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"title": "mytitle", "message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Nil(s.T(), s.notifiedMessage)
	if msgs, err := s.db.GetMessagesByApplication(7); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), msgs)
	}
}

func (s *MessageSuite) Test_CreateMessage_clientToken_appNotOwned_400() {
	s.db.User(5).NewAppWithToken(7, "other-app-token")
	auth.RegisterClient(s.ctx, s.db.User(4).NewClientWithToken(1, "client-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"appid": 7, "message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Nil(s.T(), s.notifiedMessage)
	if msgs, err := s.db.GetMessagesByApplication(7); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), msgs)
	}
}

func (s *MessageSuite) Test_CreateMessage_clientToken_unknownAppId_400() {
	auth.RegisterClient(s.ctx, s.db.User(4).NewClientWithToken(1, "client-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"appid": 999, "message": "mymessage"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Nil(s.T(), s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_basicAuth_usesBodyAppId() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	s.db.User(4).NewAppWithToken(7, "app-token")
	test.WithUser(s.ctx, 4)

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"appid": 7, "title": "mytitle", "message": "mymessage", "priority": 1}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(7)
	assert.NoError(s.T(), err)
	expected := &model.MessageExternal{ID: 1, ApplicationID: 7, Title: "mytitle", Message: "mymessage", Priority: intPtr(1), Date: t}
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), expected, toExternalMessage(msgs[0]))
	assert.Equal(s.T(), 200, s.recorder.Code)
	assert.Equal(s.T(), expected, s.notifiedMessage)
}

func (s *MessageSuite) Test_CreateMessage_appToken_ignoresBodyAppId() {
	t, _ := time.Parse("2006/01/02", "2017/01/02")
	timeNow = func() time.Time { return t }
	defer func() { timeNow = time.Now }()

	user := s.db.User(4)
	user.NewAppWithToken(7, "other-app-token")
	auth.RegisterApplication(s.ctx, user.NewAppWithToken(8, "app-token"))

	s.ctx.Request = httptest.NewRequest("POST", "/message", strings.NewReader(`{"appid": 7, "title": "mytitle", "message": "mymessage", "priority": 1}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateMessage(s.ctx)

	msgs, err := s.db.GetMessagesByApplication(8)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	if msgs7, err := s.db.GetMessagesByApplication(7); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), msgs7)
	}
	assert.Equal(s.T(), 200, s.recorder.Code)
}

func (s *MessageSuite) withURL(scheme, host, path, query string) {
	s.ctx.Request.URL = &url.URL{Path: path, RawQuery: query}
	s.ctx.Set("location", &url.URL{Scheme: scheme, Host: host})
}

func intPtr(x int) *int {
	return &x
}

// requestPage runs the given handler with a fresh context/recorder and returns
// the decoded paged response. It lets a test follow pagination links by issuing
// successive requests.
func (s *MessageSuite) requestPage(handler func(*gin.Context), userID uint, path, rawQuery string, params gin.Params) *model.PagedMessages {
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	s.ctx.Request = httptest.NewRequest("GET", "/", nil)
	s.ctx.Params = params
	s.withURL("http", "example.com", path, rawQuery)
	test.WithUser(s.ctx, userID)

	handler(s.ctx)

	require.Equal(s.T(), 200, s.recorder.Code, s.recorder.Body.String())
	var paged model.PagedMessages
	require.NoError(s.T(), json.Unmarshal(s.recorder.Body.Bytes(), &paged))
	return &paged
}

func messageIDs(msgs []*model.MessageExternal) []uint {
	ids := make([]uint, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}

func rawQueryOf(t *testing.T, rawURL string) string {
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.RawQuery
}

func (s *MessageSuite) Test_GetMessages_WithFilter_PaginationContinuity() {
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	// even ids land in app2 with high priority, odd ids in app1 with low priority
	for i := uint(1); i <= 20; i++ {
		if i%2 == 0 {
			app2.NewMessageWith(i, 8, testdb.Now)
		} else {
			app1.NewMessageWith(i, 1, testdb.Now)
		}
	}

	// priorityFrom=8 selects the even ids 2..20; page through them in steps of 3
	// and assert that each "next" link carries the filter forward.
	page1 := s.requestPage(s.a.GetMessages, 5, "/messages", "limit=3&priorityFrom=8", nil)
	assert.Equal(s.T(), []uint{20, 18, 16}, messageIDs(page1.Messages))
	assert.Equal(s.T(), uint(16), page1.Paging.Since)
	assert.Equal(s.T(), "http://example.com/messages?limit=3&priorityFrom=8&since=16", page1.Paging.Next)

	page2 := s.requestPage(s.a.GetMessages, 5, "/messages", rawQueryOf(s.T(), page1.Paging.Next), nil)
	assert.Equal(s.T(), []uint{14, 12, 10}, messageIDs(page2.Messages))
	assert.Equal(s.T(), "http://example.com/messages?limit=3&priorityFrom=8&since=10", page2.Paging.Next)

	page3 := s.requestPage(s.a.GetMessages, 5, "/messages", rawQueryOf(s.T(), page2.Paging.Next), nil)
	assert.Equal(s.T(), []uint{8, 6, 4}, messageIDs(page3.Messages))
	assert.Equal(s.T(), "http://example.com/messages?limit=3&priorityFrom=8&since=4", page3.Paging.Next)

	page4 := s.requestPage(s.a.GetMessages, 5, "/messages", rawQueryOf(s.T(), page3.Paging.Next), nil)
	assert.Equal(s.T(), []uint{2}, messageIDs(page4.Messages))
	assert.Empty(s.T(), page4.Paging.Next, "last page must not advertise a next link")
}

func (s *MessageSuite) Test_GetMessages_WithFilter_MultiCondition_PreservedInNext() {
	day := func(n int) time.Time { return testdb.Now.AddDate(0, 0, n) }
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	app1.NewMessageWith(1, 10, day(1))
	app2.NewMessageWith(2, 3, day(2))
	app2.NewMessageWith(3, 7, day(3))
	app1.NewMessageWith(4, 8, day(4))
	app2.NewMessageWith(5, 9, day(5))

	// app2 AND priority>=5 AND day(3)<=date<=day(5) -> ids 5 then 3
	query := "appid=2&priorityFrom=5&dateFrom=" + day(3).Format(time.RFC3339) +
		"&dateUntil=" + day(5).Format(time.RFC3339) + "&limit=1"
	page1 := s.requestPage(s.a.GetMessages, 5, "/messages", query, nil)
	assert.Equal(s.T(), []uint{5}, messageIDs(page1.Messages))

	u, err := url.Parse(page1.Paging.Next)
	require.NoError(s.T(), err)
	q := u.Query()
	assert.Equal(s.T(), "2", q.Get("appid"))
	assert.Equal(s.T(), "5", q.Get("priorityFrom"))
	assert.Equal(s.T(), day(3).Format(time.RFC3339), q.Get("dateFrom"))
	assert.Equal(s.T(), day(5).Format(time.RFC3339), q.Get("dateUntil"))
	assert.Equal(s.T(), "1", q.Get("limit"))
	assert.Equal(s.T(), "5", q.Get("since"))

	// following the next link keeps the filter applied
	page2 := s.requestPage(s.a.GetMessages, 5, "/messages", u.RawQuery, nil)
	assert.Equal(s.T(), []uint{3}, messageIDs(page2.Messages))
	assert.Empty(s.T(), page2.Paging.Next)
}

func (s *MessageSuite) Test_GetMessages_WithMultipleAppIDs() {
	user := s.db.User(5)
	user.App(1).NewMessageWith(1, 0, testdb.Now)
	user.App(2).NewMessageWith(2, 0, testdb.Now)
	user.App(3).NewMessageWith(3, 0, testdb.Now)

	page := s.requestPage(s.a.GetMessages, 5, "/messages", "appid=1&appid=3", nil)
	assert.Equal(s.T(), []uint{3, 1}, messageIDs(page.Messages))
}

func (s *MessageSuite) Test_GetMessagesWithApplication_WithFilter() {
	user := s.db.User(5)
	app2 := user.App(2)
	app2.NewMessageWith(1, 1, testdb.Now)
	app2.NewMessageWith(2, 8, testdb.Now)
	app2.NewMessageWith(3, 10, testdb.Now)

	page := s.requestPage(s.a.GetMessagesWithApplication, 5, "/application/2/message", "priorityFrom=8",
		gin.Params{{Key: "id", Value: "2"}})
	assert.Equal(s.T(), []uint{3, 2}, messageIDs(page.Messages))
}

func (s *MessageSuite) Test_GetMessagesWithApplication_IgnoresAppidQuery() {
	user := s.db.User(5)
	app1 := user.App(1)
	app2 := user.App(2)
	app1.NewMessageWith(1, 5, testdb.Now)
	app2.NewMessageWith(2, 5, testdb.Now)
	app2.NewMessageWith(3, 5, testdb.Now)

	// appid=1 in the query is meaningless on this endpoint: the path id (2) wins,
	// and appid must not leak into the next link.
	page := s.requestPage(s.a.GetMessagesWithApplication, 5, "/application/2/message", "appid=1&limit=1",
		gin.Params{{Key: "id", Value: "2"}})
	assert.Equal(s.T(), []uint{3}, messageIDs(page.Messages))

	u, err := url.Parse(page.Paging.Next)
	require.NoError(s.T(), err)
	assert.NotContains(s.T(), u.Query(), "appid")
	assert.Equal(s.T(), "3", u.Query().Get("since"))
}

func (s *MessageSuite) Test_GetMessages_BadPriorityFilter_400() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/messages", "priorityFrom=abc")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_GetMessages_BadDateFilter_400() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/messages", "dateFrom=notadate")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *MessageSuite) Test_GetMessages_InvalidLimitWinsOverValidFilter_400() {
	s.db.User(5)
	test.WithUser(s.ctx, 5)
	s.withURL("http", "example.com", "/messages", "limit=555&priorityFrom=8")
	s.a.GetMessages(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}
