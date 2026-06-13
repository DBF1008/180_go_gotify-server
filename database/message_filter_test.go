package database

import (
	"time"

	"github.com/gotify/server/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uintPtr(v uint) *uint     { return &v }
func intPtr(v int) *int        { return &v }
func timePtr(v time.Time) *time.Time { return &v }

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_NoFilter() {
	user := &model.User{Name: "test_filter_user", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app1 := &model.Application{UserID: user.ID, Token: "Af000000001", Name: "app1"}
	app2 := &model.Application{UserID: user.ID, Token: "Af000000002", Name: "app2"}
	require.NoError(s.T(), s.db.CreateApplication(app1))
	require.NoError(s.T(), s.db.CreateApplication(app2))

	for i := 1; i <= 10; i++ {
		s.db.CreateMessage(&model.Message{
			ApplicationID: app1.ID,
			Message:       "msg",
			Priority:      i % 5,
			Date:          now.Add(time.Duration(i) * time.Second),
		})
		s.db.CreateMessage(&model.Message{
			ApplicationID: app2.ID,
			Message:       "msg",
			Priority:      i % 3,
			Date:          now.Add(time.Duration(i) * time.Second),
		})
	}

	// No filter should return all 20 messages
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, nil)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 20)

	// No filter with empty MessageFilter should also return all
	msgs, err = s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, &model.MessageFilter{})
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 20)
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_ByApplication() {
	user := &model.User{Name: "test_filter_app", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app1 := &model.Application{UserID: user.ID, Token: "Af000000003", Name: "app1"}
	app2 := &model.Application{UserID: user.ID, Token: "Af000000004", Name: "app2"}
	require.NoError(s.T(), s.db.CreateApplication(app1))
	require.NoError(s.T(), s.db.CreateApplication(app2))

	for i := 1; i <= 5; i++ {
		s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "a1", Date: now})
		s.db.CreateMessage(&model.Message{ApplicationID: app2.ID, Message: "a2", Date: now})
	}

	filter := &model.MessageFilter{ApplicationID: uintPtr(app1.ID)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 5)
	for _, m := range msgs {
		assert.Equal(s.T(), app1.ID, m.ApplicationID)
	}
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_ByPriorityExact() {
	user := &model.User{Name: "test_filter_prio", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000005", Name: "prioapp"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	priorities := []int{0, 1, 3, 5, 5, 7, 10}
	for _, p := range priorities {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "msg", Priority: p, Date: now})
	}

	filter := &model.MessageFilter{Priority: intPtr(5)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	for _, m := range msgs {
		assert.Equal(s.T(), 5, m.Priority)
	}

	// priority=0 should work (zero is valid)
	filter0 := &model.MessageFilter{Priority: intPtr(0)}
	msgs0, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter0)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs0, 1)
	assert.Equal(s.T(), 0, msgs0[0].Priority)
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_ByPriorityRange() {
	user := &model.User{Name: "test_filter_prange", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000006", Name: "rangeapp"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	for _, p := range []int{0, 1, 3, 5, 7, 10} {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "msg", Priority: p, Date: now})
	}

	filter := &model.MessageFilter{PriorityMin: intPtr(3), PriorityMax: intPtr(7)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 3) // 3, 5, 7
	for _, m := range msgs {
		assert.GreaterOrEqual(s.T(), m.Priority, 3)
		assert.LessOrEqual(s.T(), m.Priority, 7)
	}
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_ByTimeWindow() {
	user := &model.User{Name: "test_filter_time", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000007", Name: "timeapp"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	t1 := now.Add(-3 * time.Hour)
	t2 := now.Add(-2 * time.Hour)
	t3 := now.Add(-1 * time.Hour)
	t4 := now

	for _, t := range []time.Time{t1, t2, t3, t4} {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "msg", Date: t})
	}

	// after t1, before t4 → should get t2, t3
	filter := &model.MessageFilter{After: timePtr(t1), Before: timePtr(t4)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	for _, m := range msgs {
		assert.True(s.T(), m.Date.After(t1))
		assert.True(s.T(), m.Date.Before(t4))
	}
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_CombinedConditions() {
	user := &model.User{Name: "test_filter_combo", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app1 := &model.Application{UserID: user.ID, Token: "Af000000008", Name: "combo1"}
	app2 := &model.Application{UserID: user.ID, Token: "Af000000009", Name: "combo2"}
	require.NoError(s.T(), s.db.CreateApplication(app1))
	require.NoError(s.T(), s.db.CreateApplication(app2))

	baseTime := now.Add(-1 * time.Hour)

	// app1: high priority, recent
	s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "hit1", Priority: 8, Date: now})
	s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "hit2", Priority: 6, Date: now})
	// app1: low priority, recent (excluded by priority)
	s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "miss1", Priority: 2, Date: now})
	// app2: high priority, recent (excluded by appid)
	s.db.CreateMessage(&model.Message{ApplicationID: app2.ID, Message: "miss2", Priority: 9, Date: now})
	// app1: high priority, old (excluded by time)
	s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "miss3", Priority: 7, Date: baseTime})

	filter := &model.MessageFilter{
		ApplicationID: uintPtr(app1.ID),
		PriorityMin:   intPtr(5),
		After:         timePtr(baseTime),
	}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 2)
	for _, m := range msgs {
		assert.Equal(s.T(), app1.ID, m.ApplicationID)
		assert.GreaterOrEqual(s.T(), m.Priority, 5)
		assert.True(s.T(), m.Date.After(baseTime))
	}
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_EmptyResult() {
	user := &model.User{Name: "test_filter_empty", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000010", Name: "emptyapp"}
	require.NoError(s.T(), s.db.CreateApplication(app))
	s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "low", Priority: 1, Date: now})

	filter := &model.MessageFilter{PriorityMin: intPtr(10)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), msgs)
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_WithSinceAndLimit() {
	user := &model.User{Name: "test_filter_page", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000011", Name: "pageapp"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	// Create 20 high-priority messages
	for i := 1; i <= 20; i++ {
		s.db.CreateMessage(&model.Message{
			ApplicationID: app.ID,
			Message:       "msg",
			Priority:      5,
			Date:          now.Add(time.Duration(i) * time.Second),
		})
	}

	// First page: limit=5, no since
	filter := &model.MessageFilter{Priority: intPtr(5)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user.ID, 5, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 5)

	// Second page: since = last ID from first page
	since := msgs[len(msgs)-1].ID
	msgs2, err := s.db.GetMessagesByUserWithFilter(user.ID, 5, since, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs2, 5)

	// Ensure no overlap between pages
	lastPage1ID := msgs[len(msgs)-1].ID
	firstPage2ID := msgs2[0].ID
	assert.True(s.T(), firstPage2ID < lastPage1ID)
}

func (s *DatabaseSuite) TestGetMessagesByApplicationWithFilter_NoFilter() {
	user := &model.User{Name: "test_appfilter_nf", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000012", Name: "appfilter"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	for i := 1; i <= 5; i++ {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "msg", Priority: i, Date: now})
	}

	msgs, err := s.db.GetMessagesByApplicationWithFilter(app.ID, 100, 0, nil)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 5)
}

func (s *DatabaseSuite) TestGetMessagesByApplicationWithFilter_ByPriority() {
	user := &model.User{Name: "test_appfilter_p", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000013", Name: "appfilterp"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	for _, p := range []int{0, 2, 4, 6, 8} {
		s.db.CreateMessage(&model.Message{ApplicationID: app.ID, Message: "msg", Priority: p, Date: now})
	}

	filter := &model.MessageFilter{PriorityMin: intPtr(4)}
	msgs, err := s.db.GetMessagesByApplicationWithFilter(app.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 3) // 4, 6, 8
	for _, m := range msgs {
		assert.GreaterOrEqual(s.T(), m.Priority, 4)
	}
}

func (s *DatabaseSuite) TestGetMessagesByApplicationWithFilter_CombinedWithSince() {
	user := &model.User{Name: "test_appfilter_cs", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user))

	app := &model.Application{UserID: user.ID, Token: "Af000000014", Name: "appfiltercs"}
	require.NoError(s.T(), s.db.CreateApplication(app))

	for i := 1; i <= 10; i++ {
		s.db.CreateMessage(&model.Message{
			ApplicationID: app.ID,
			Message:       "msg",
			Priority:      i,
			Date:          now.Add(time.Duration(i) * time.Second),
		})
	}

	filter := &model.MessageFilter{PriorityMin: intPtr(5)}
	msgs, err := s.db.GetMessagesByApplicationWithFilter(app.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 6) // priorities 5,6,7,8,9,10

	// Now with since
	since := msgs[2].ID // use 3rd message as cursor
	msgs2, err := s.db.GetMessagesByApplicationWithFilter(app.ID, 100, since, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs2, 3) // remaining: priorities 5,6,7
	for _, m := range msgs2 {
		assert.GreaterOrEqual(s.T(), m.Priority, 5)
		assert.True(s.T(), m.ID < since)
	}
}

func (s *DatabaseSuite) TestGetMessagesByUserWithFilter_CrossUserIsolation() {
	user1 := &model.User{Name: "test_iso_u1", Pass: []byte{1}}
	user2 := &model.User{Name: "test_iso_u2", Pass: []byte{1}}
	require.NoError(s.T(), s.db.CreateUser(user1))
	require.NoError(s.T(), s.db.CreateUser(user2))

	app1 := &model.Application{UserID: user1.ID, Token: "Af000000015", Name: "u1app"}
	app2 := &model.Application{UserID: user2.ID, Token: "Af000000016", Name: "u2app"}
	require.NoError(s.T(), s.db.CreateApplication(app1))
	require.NoError(s.T(), s.db.CreateApplication(app2))

	s.db.CreateMessage(&model.Message{ApplicationID: app1.ID, Message: "u1msg", Priority: 10, Date: now})
	s.db.CreateMessage(&model.Message{ApplicationID: app2.ID, Message: "u2msg", Priority: 10, Date: now})

	// Query for user1 with high priority filter should only get user1's message
	filter := &model.MessageFilter{PriorityMin: intPtr(5)}
	msgs, err := s.db.GetMessagesByUserWithFilter(user1.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs, 1)
	assert.Equal(s.T(), app1.ID, msgs[0].ApplicationID)

	// Query for user2 should only get user2's message
	msgs2, err := s.db.GetMessagesByUserWithFilter(user2.ID, 100, 0, filter)
	require.NoError(s.T(), err)
	assert.Len(s.T(), msgs2, 1)
	assert.Equal(s.T(), app2.ID, msgs2[0].ApplicationID)
}
