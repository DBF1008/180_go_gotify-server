package database

import (
	"testing"
	"time"

	"github.com/gotify/server/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *DatabaseSuite) TestMessage() {
	messages, err := s.db.GetMessageByID(5)
	require.NoError(s.T(), err)
	assert.Nil(s.T(), messages, "not existing message")

	user := &model.User{Name: "test", Pass: []byte{1}}
	s.db.CreateUser(user)
	assert.NotEqual(s.T(), 0, user.ID)

	backupServer := &model.Application{UserID: user.ID, Token: "A0000000000", Name: "backupserver"}
	s.db.CreateApplication(backupServer)
	assert.NotEqual(s.T(), 0, backupServer.ID)

	msgs, err := s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)

	msgs, err = s.db.GetMessagesByApplication(backupServer.ID)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)

	backupdone := &model.Message{ApplicationID: backupServer.ID, Message: "backup done", Title: "backup", Priority: 1, Date: time.Now()}
	require.NoError(s.T(), s.db.CreateMessage(backupdone))
	assert.NotEqual(s.T(), 0, backupdone.ID)

	messages, err = s.db.GetMessageByID(backupdone.ID)
	require.NoError(s.T(), err)
	assertEquals(s.T(), messages, backupdone)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assertEquals(s.T(), msgs[0], backupdone)

	msgs, err = s.db.GetMessagesByApplication(backupServer.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assertEquals(s.T(), msgs[0], backupdone)

	loginServer := &model.Application{UserID: user.ID, Token: "A0000000001", Name: "loginserver"}
	require.NoError(s.T(), s.db.CreateApplication(loginServer))
	assert.NotEqual(s.T(), 0, loginServer.ID)

	logindone := &model.Message{ApplicationID: loginServer.ID, Message: "login done", Title: "login", Priority: 1, Date: time.Now()}
	require.NoError(s.T(), s.db.CreateMessage(logindone))
	assert.NotEqual(s.T(), 0, logindone.ID)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	assertEquals(s.T(), msgs[0], logindone)
	assertEquals(s.T(), msgs[1], backupdone)

	msgs, err = s.db.GetMessagesByApplication(backupServer.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assertEquals(s.T(), msgs[0], backupdone)

	loginfailed := &model.Message{ApplicationID: loginServer.ID, Message: "login failed", Title: "login", Priority: 1, Date: time.Now()}
	require.NoError(s.T(), s.db.CreateMessage(loginfailed))
	assert.NotEqual(s.T(), 0, loginfailed.ID)

	msgs, err = s.db.GetMessagesByApplication(backupServer.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assertEquals(s.T(), msgs[0], backupdone)

	msgs, err = s.db.GetMessagesByApplication(loginServer.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	assertEquals(s.T(), msgs[0], loginfailed)
	assertEquals(s.T(), msgs[1], logindone)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 3)
	assertEquals(s.T(), msgs[0], loginfailed)
	assertEquals(s.T(), msgs[1], logindone)
	assertEquals(s.T(), msgs[2], backupdone)

	backupfailed := &model.Message{ApplicationID: backupServer.ID, Message: "backup failed", Title: "backup", Priority: 1, Date: time.Now()}
	require.NoError(s.T(), s.db.CreateMessage(backupfailed))
	assert.NotEqual(s.T(), 0, backupfailed.ID)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 4)
	assertEquals(s.T(), msgs[0], backupfailed)
	assertEquals(s.T(), msgs[1], loginfailed)
	assertEquals(s.T(), msgs[2], logindone)
	assertEquals(s.T(), msgs[3], backupdone)

	msgs, err = s.db.GetMessagesByApplication(loginServer.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	assertEquals(s.T(), msgs[0], loginfailed)
	assertEquals(s.T(), msgs[1], logindone)

	require.NoError(s.T(), s.db.DeleteMessagesByApplication(loginServer.ID))
	msgs, err = s.db.GetMessagesByApplication(loginServer.ID)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	assertEquals(s.T(), msgs[0], backupfailed)
	assertEquals(s.T(), msgs[1], backupdone)

	logindone = &model.Message{ApplicationID: loginServer.ID, Message: "login done", Title: "login", Priority: 1, Date: time.Now()}
	require.NoError(s.T(), s.db.CreateMessage(logindone))
	assert.NotEqual(s.T(), 0, logindone.ID)

	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 3)
	assertEquals(s.T(), msgs[0], logindone)
	assertEquals(s.T(), msgs[1], backupfailed)
	assertEquals(s.T(), msgs[2], backupdone)

	s.db.DeleteMessagesByUser(user.ID)
	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)

	logout := &model.Message{ApplicationID: loginServer.ID, Message: "logout success", Title: "logout", Priority: 1, Date: time.Now()}
	s.db.CreateMessage(logout)
	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assertEquals(s.T(), msgs[0], logout)

	require.NoError(s.T(), s.db.DeleteMessageByID(logout.ID))
	msgs, err = s.db.GetMessagesByUser(user.ID)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)
}

func (s *DatabaseSuite) TestGetMessagesSince() {
	user := &model.User{Name: "test", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "A0000000000"}
	app2 := &model.Application{UserID: user.ID, Token: "A0000000001"}
	require.NoError(s.T(), s.db.CreateApplication(app))
	require.NoError(s.T(), s.db.CreateApplication(app2))

	curDate := time.Now()
	for i := 1; i <= 500; i++ {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "abc", Date: curDate.Add(time.Duration(i) * time.Second)})
		s.db.CreateMessage(&model.Message{ApplicationID: app2.ID, Message: "abc", Date: curDate.Add(time.Duration(i) * time.Second)})
	}

	actual, err := s.db.GetMessagesByUserSince(user.ID, 50, 0)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 1000, 951, 1)

	actual, err = s.db.GetMessagesByUserSince(user.ID, 50, 951)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 950, 901, 1)

	actual, err = s.db.GetMessagesByUserSince(user.ID, 100, 951)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 100)
	hasIDInclusiveBetween(s.T(), actual, 950, 851, 1)

	actual, err = s.db.GetMessagesByUserSince(user.ID, 100, 51)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 50, 1, 1)

	actual, err = s.db.GetMessagesByApplicationSince(app.ID, 50, 0)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 999, 901, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app.ID, 50, 901)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 899, 801, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app.ID, 100, 666)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 100)
	hasIDInclusiveBetween(s.T(), actual, 665, 467, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app.ID, 100, 101)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 99, 1, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app2.ID, 50, 0)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 1000, 902, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app2.ID, 50, 902)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 900, 802, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app2.ID, 100, 667)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 100)
	hasIDInclusiveBetween(s.T(), actual, 666, 468, 2)

	actual, err = s.db.GetMessagesByApplicationSince(app2.ID, 100, 102)
	require.NoError(s.T(), err)
	assert.Len(s.T(), actual, 50)
	hasIDInclusiveBetween(s.T(), actual, 100, 2, 2)
}

func hasIDInclusiveBetween(t *testing.T, msgs []*model.Message, from, to, decrement int) {
	index := 0
	for expectedID := from; expectedID >= to; expectedID -= decrement {
		if !assert.Equal(t, uint(expectedID), msgs[index].ID) {
			break
		}
		index++
	}
	assert.Equal(t, index, len(msgs), "not all entries inside msgs were checked")
}

// assertEquals compares messages and correctly check dates.
func assertEquals(t *testing.T, left, right *model.Message) {
	assert.Equal(t, left.Date.Unix(), right.Date.Unix())
	left.Date = right.Date
	assert.Equal(t, left, right)
}

func ptrTo[T any](v T) *T { return &v }

func assertMessageIDs(t *testing.T, msgs []*model.Message, ids ...uint) {
	actual := make([]uint, len(msgs))
	for i, m := range msgs {
		actual[i] = m.ID
	}
	if ids == nil {
		ids = []uint{}
	}
	assert.Equal(t, ids, actual)
}

func (s *DatabaseSuite) TestGetMessagesWithFilter() {
	day := func(n int) time.Time { return now.AddDate(0, 0, n) }

	user1 := &model.User{Name: "u1", Pass: []byte{1}}
	user2 := &model.User{Name: "u2", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user1))
	require.NoError(s.T(), s.db.CreateUser(user2))

	require.NoError(s.T(), s.db.CreateApplication(&model.Application{ID: 1, UserID: user1.ID, Token: "A1"}))
	require.NoError(s.T(), s.db.CreateApplication(&model.Application{ID: 2, UserID: user1.ID, Token: "A2"}))
	require.NoError(s.T(), s.db.CreateApplication(&model.Application{ID: 3, UserID: user2.ID, Token: "A3"}))
	require.NoError(s.T(), s.db.CreateApplication(&model.Application{ID: 4, UserID: user1.ID, Token: "A4", Internal: true}))

	messages := []*model.Message{
		{ID: 1, ApplicationID: 1, Priority: 1, Date: day(1)},
		{ID: 2, ApplicationID: 1, Priority: 5, Date: day(2)},
		{ID: 3, ApplicationID: 1, Priority: 10, Date: day(3)},
		{ID: 4, ApplicationID: 2, Priority: 2, Date: day(4)},
		{ID: 5, ApplicationID: 2, Priority: 8, Date: day(5)},
		{ID: 6, ApplicationID: 2, Priority: 10, Date: day(6)},
		{ID: 7, ApplicationID: 3, Priority: 10, Date: day(7)}, // user2
		{ID: 8, ApplicationID: 4, Priority: 7, Date: day(8)}, // user1 internal
	}
	for _, m := range messages {
		require.NoError(s.T(), s.db.CreateMessage(m))
	}

	get := func(filter *model.MessageFilter) []*model.Message {
		msgs, err := s.db.GetMessagesByUserWithFilter(user1.ID, filter, 100, 0)
		require.NoError(s.T(), err)
		return msgs
	}

	s.Run("nil filter returns all of the user's messages", func() {
		assertMessageIDs(s.T(), get(nil), 8, 6, 5, 4, 3, 2, 1)
	})
	s.Run("priorityFrom", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{PriorityFrom: ptrTo(8)}), 6, 5, 3)
	})
	s.Run("priorityUntil", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{PriorityUntil: ptrTo(2)}), 4, 1)
	})
	s.Run("priority range", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{PriorityFrom: ptrTo(5), PriorityUntil: ptrTo(9)}), 8, 5, 2)
	})
	s.Run("dateFrom", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{DateFrom: ptrTo(day(5))}), 8, 6, 5)
	})
	s.Run("dateUntil", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{DateUntil: ptrTo(day(3))}), 3, 2, 1)
	})
	s.Run("date window", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{DateFrom: ptrTo(day(2)), DateUntil: ptrTo(day(5))}), 5, 4, 3, 2)
	})
	s.Run("single appid", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{AppIDs: []uint{2}}), 6, 5, 4)
	})
	s.Run("multiple appids", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{AppIDs: []uint{1, 4}}), 8, 3, 2, 1)
	})
	s.Run("internal application is queryable", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{AppIDs: []uint{4}}), 8)
	})
	s.Run("multi-condition AND", func() {
		filter := &model.MessageFilter{AppIDs: []uint{2}, PriorityFrom: ptrTo(8), DateFrom: ptrTo(day(6))}
		assertMessageIDs(s.T(), get(filter), 6)
	})
	s.Run("filtering another user's application returns nothing", func() {
		assertMessageIDs(s.T(), get(&model.MessageFilter{AppIDs: []uint{3}}))
	})

	s.Run("since cursor combined with filter paginates", func() {
		filter := &model.MessageFilter{PriorityFrom: ptrTo(8)} // matches ids 3, 5, 6
		page1, err := s.db.GetMessagesByUserWithFilter(user1.ID, filter, 2, 0)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), page1, 6, 5)

		page2, err := s.db.GetMessagesByUserWithFilter(user1.ID, filter, 2, page1[len(page1)-1].ID)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), page2, 3)

		page3, err := s.db.GetMessagesByUserWithFilter(user1.ID, filter, 2, page2[len(page2)-1].ID)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), page3)
	})

	s.Run("application query applies filter", func() {
		msgs, err := s.db.GetMessagesByApplicationWithFilter(2, &model.MessageFilter{PriorityFrom: ptrTo(8)}, 100, 0)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), msgs, 6, 5)
	})
	s.Run("application query ignores appid filter", func() {
		// AppIDs is meaningless for the application-scoped query; passing app 1
		// must not change the result, which stays scoped to app 2.
		msgs, err := s.db.GetMessagesByApplicationWithFilter(2, &model.MessageFilter{AppIDs: []uint{1}}, 100, 0)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), msgs, 6, 5, 4)
	})
	s.Run("application query since cursor combined with filter", func() {
		filter := &model.MessageFilter{PriorityFrom: ptrTo(8)} // app 2 -> ids 5, 6
		page1, err := s.db.GetMessagesByApplicationWithFilter(2, filter, 1, 0)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), page1, 6)

		page2, err := s.db.GetMessagesByApplicationWithFilter(2, filter, 1, page1[len(page1)-1].ID)
		require.NoError(s.T(), err)
		assertMessageIDs(s.T(), page2, 5)
	})
}
