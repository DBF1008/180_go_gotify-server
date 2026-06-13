package model

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mfIntPtr(v int) *int { return &v }

func mfTimePtr(v time.Time) *time.Time { return &v }

func mfMessage(appID uint, priority int, date time.Time) *MessageExternal {
	p := priority
	return &MessageExternal{ApplicationID: appID, Priority: &p, Date: date}
}

func TestMessageFilter_Matches(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 6, d, 0, 0, 0, 0, time.UTC) }

	tests := []struct {
		name   string
		filter *MessageFilter
		msg    *MessageExternal
		want   bool
	}{
		{"nil filter matches", nil, mfMessage(1, 5, day(10)), true},
		{"empty filter matches", &MessageFilter{}, mfMessage(1, 5, day(10)), true},
		{"nil message matches", &MessageFilter{PriorityFrom: mfIntPtr(5)}, nil, true},

		{"appid in single set", &MessageFilter{AppIDs: []uint{2}}, mfMessage(2, 0, day(10)), true},
		{"appid not in single set", &MessageFilter{AppIDs: []uint{2}}, mfMessage(3, 0, day(10)), false},
		{"appid in multi set", &MessageFilter{AppIDs: []uint{2, 7, 9}}, mfMessage(7, 0, day(10)), true},
		{"appid not in multi set", &MessageFilter{AppIDs: []uint{2, 7, 9}}, mfMessage(5, 0, day(10)), false},

		{"priorityFrom equal boundary", &MessageFilter{PriorityFrom: mfIntPtr(5)}, mfMessage(1, 5, day(10)), true},
		{"priorityFrom above", &MessageFilter{PriorityFrom: mfIntPtr(5)}, mfMessage(1, 6, day(10)), true},
		{"priorityFrom below", &MessageFilter{PriorityFrom: mfIntPtr(5)}, mfMessage(1, 4, day(10)), false},
		{"priorityUntil equal boundary", &MessageFilter{PriorityUntil: mfIntPtr(5)}, mfMessage(1, 5, day(10)), true},
		{"priorityUntil below", &MessageFilter{PriorityUntil: mfIntPtr(5)}, mfMessage(1, 4, day(10)), true},
		{"priorityUntil above", &MessageFilter{PriorityUntil: mfIntPtr(5)}, mfMessage(1, 6, day(10)), false},
		{"priority range inside", &MessageFilter{PriorityFrom: mfIntPtr(3), PriorityUntil: mfIntPtr(7)}, mfMessage(1, 5, day(10)), true},
		{"priority range outside", &MessageFilter{PriorityFrom: mfIntPtr(3), PriorityUntil: mfIntPtr(7)}, mfMessage(1, 8, day(10)), false},
		{"priority from greater than until never matches", &MessageFilter{PriorityFrom: mfIntPtr(7), PriorityUntil: mfIntPtr(3)}, mfMessage(1, 5, day(10)), false},
		{"negative priority within bounds", &MessageFilter{PriorityFrom: mfIntPtr(-5), PriorityUntil: mfIntPtr(-1)}, mfMessage(1, -3, day(10)), true},

		{"dateFrom equal boundary", &MessageFilter{DateFrom: mfTimePtr(day(10))}, mfMessage(1, 0, day(10)), true},
		{"dateFrom after", &MessageFilter{DateFrom: mfTimePtr(day(10))}, mfMessage(1, 0, day(11)), true},
		{"dateFrom before", &MessageFilter{DateFrom: mfTimePtr(day(10))}, mfMessage(1, 0, day(9)), false},
		{"dateUntil equal boundary", &MessageFilter{DateUntil: mfTimePtr(day(10))}, mfMessage(1, 0, day(10)), true},
		{"dateUntil before", &MessageFilter{DateUntil: mfTimePtr(day(10))}, mfMessage(1, 0, day(9)), true},
		{"dateUntil after", &MessageFilter{DateUntil: mfTimePtr(day(10))}, mfMessage(1, 0, day(11)), false},
		{"date window inside", &MessageFilter{DateFrom: mfTimePtr(day(5)), DateUntil: mfTimePtr(day(15))}, mfMessage(1, 0, day(10)), true},
		{"date window outside", &MessageFilter{DateFrom: mfTimePtr(day(5)), DateUntil: mfTimePtr(day(15))}, mfMessage(1, 0, day(20)), false},

		{
			"all dimensions match",
			&MessageFilter{AppIDs: []uint{2, 7}, PriorityFrom: mfIntPtr(5), PriorityUntil: mfIntPtr(9), DateFrom: mfTimePtr(day(5)), DateUntil: mfTimePtr(day(15))},
			mfMessage(7, 6, day(10)),
			true,
		},
		{
			"all dimensions but appid fails",
			&MessageFilter{AppIDs: []uint{2, 7}, PriorityFrom: mfIntPtr(5), PriorityUntil: mfIntPtr(9), DateFrom: mfTimePtr(day(5)), DateUntil: mfTimePtr(day(15))},
			mfMessage(3, 6, day(10)),
			false,
		},
		{
			"all dimensions but date fails",
			&MessageFilter{AppIDs: []uint{2, 7}, PriorityFrom: mfIntPtr(5), PriorityUntil: mfIntPtr(9), DateFrom: mfTimePtr(day(5)), DateUntil: mfTimePtr(day(15))},
			mfMessage(7, 6, day(20)),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.filter.Matches(tt.msg))
		})
	}
}

func TestMessageFilter_Matches_NilPriorityTreatedAsZero(t *testing.T) {
	msg := &MessageExternal{ApplicationID: 1, Date: time.Now()} // Priority nil

	assert.True(t, (&MessageFilter{PriorityFrom: mfIntPtr(0)}).Matches(msg))
	assert.True(t, (&MessageFilter{PriorityUntil: mfIntPtr(0)}).Matches(msg))
	assert.False(t, (&MessageFilter{PriorityFrom: mfIntPtr(1)}).Matches(msg))
	assert.False(t, (&MessageFilter{PriorityUntil: mfIntPtr(-1)}).Matches(msg))
}

func TestParseMessageFilter(t *testing.T) {
	t.Run("empty query yields empty non-nil filter", func(t *testing.T) {
		filter, err := ParseMessageFilter(url.Values{})
		require.NoError(t, err)
		require.NotNil(t, filter)
		assert.Empty(t, filter.AppIDs)
		assert.Nil(t, filter.PriorityFrom)
		assert.Nil(t, filter.PriorityUntil)
		assert.Nil(t, filter.DateFrom)
		assert.Nil(t, filter.DateUntil)
	})

	t.Run("all parameters parsed", func(t *testing.T) {
		values := url.Values{
			"appid":         {"2", "7"},
			"priorityFrom":  {"3"},
			"priorityUntil": {"9"},
			"dateFrom":      {"2026-06-01T00:00:00Z"},
			"dateUntil":     {"2026-06-30T23:59:59Z"},
		}
		filter, err := ParseMessageFilter(values)
		require.NoError(t, err)
		assert.Equal(t, []uint{2, 7}, filter.AppIDs)
		assert.Equal(t, 3, *filter.PriorityFrom)
		assert.Equal(t, 9, *filter.PriorityUntil)
		assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *filter.DateFrom)
		assert.Equal(t, time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC), *filter.DateUntil)
	})

	t.Run("empty appid values are skipped", func(t *testing.T) {
		filter, err := ParseMessageFilter(url.Values{"appid": {"", "5", ""}})
		require.NoError(t, err)
		assert.Equal(t, []uint{5}, filter.AppIDs)
	})

	t.Run("negative priority parsed", func(t *testing.T) {
		filter, err := ParseMessageFilter(url.Values{"priorityFrom": {"-3"}})
		require.NoError(t, err)
		assert.Equal(t, -3, *filter.PriorityFrom)
	})

	errorCases := map[string]url.Values{
		"invalid appid":         {"appid": {"abc"}},
		"invalid priorityFrom":  {"priorityFrom": {"high"}},
		"invalid priorityUntil": {"priorityUntil": {"3.5"}},
		"invalid dateFrom":      {"dateFrom": {"notadate"}},
		"invalid dateUntil":     {"dateUntil": {"2026-13-40T00:00:00Z"}},
	}
	for name, values := range errorCases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseMessageFilter(values)
			assert.Error(t, err)
		})
	}
}

func TestMessageFilter_AddToQuery(t *testing.T) {
	t.Run("nil receiver is a no-op", func(t *testing.T) {
		query := url.Values{}
		var filter *MessageFilter
		filter.AddToQuery(query)
		assert.Empty(t, query)
	})

	t.Run("empty filter adds nothing", func(t *testing.T) {
		query := url.Values{}
		(&MessageFilter{}).AddToQuery(query)
		assert.Empty(t, query)
	})

	t.Run("all fields written", func(t *testing.T) {
		query := url.Values{}
		filter := &MessageFilter{
			AppIDs:        []uint{2, 7},
			PriorityFrom:  mfIntPtr(3),
			PriorityUntil: mfIntPtr(9),
			DateFrom:      mfTimePtr(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
			DateUntil:     mfTimePtr(time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)),
		}
		filter.AddToQuery(query)
		assert.Equal(t, []string{"2", "7"}, query["appid"])
		assert.Equal(t, "3", query.Get("priorityFrom"))
		assert.Equal(t, "9", query.Get("priorityUntil"))
		assert.Equal(t, "2026-06-01T00:00:00Z", query.Get("dateFrom"))
		assert.Equal(t, "2026-06-30T23:59:59Z", query.Get("dateUntil"))
	})
}

func TestMessageFilter_ParseAddToQueryRoundTrip(t *testing.T) {
	original := url.Values{
		"appid":         {"2", "7"},
		"priorityFrom":  {"3"},
		"priorityUntil": {"9"},
		"dateFrom":      {"2026-06-01T00:00:00Z"},
		"dateUntil":     {"2026-06-30T23:59:59Z"},
	}

	filter, err := ParseMessageFilter(original)
	require.NoError(t, err)

	roundTripped := url.Values{}
	filter.AddToQuery(roundTripped)

	assert.Equal(t, original, roundTripped)
	// Encode sorts keys deterministically, so paging links are stable.
	assert.Equal(t, original.Encode(), roundTripped.Encode())
}
