package model

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// MessageFilter holds the structured criteria for selecting messages.
//
// It is the single filtering vocabulary shared across the project: the REST
// query handlers translate it to SQL (see database.GetMessagesByUserWithFilter
// and database.GetMessagesByApplicationWithFilter), while the WebSocket stream
// and plugin delivery evaluate it in-memory via Matches. Only content criteria
// live here; pagination (limit/since) is handled separately by the REST layer.
//
// All bounds are inclusive. A zero-valued MessageFilter (or a nil one) matches
// everything.
type MessageFilter struct {
	// AppIDs restricts results to these application ids. Empty means all
	// applications the requester is allowed to see.
	AppIDs []uint
	// PriorityFrom, when set, keeps only messages with priority >= *PriorityFrom.
	PriorityFrom *int
	// PriorityUntil, when set, keeps only messages with priority <= *PriorityUntil.
	PriorityUntil *int
	// DateFrom, when set, keeps only messages created at or after *DateFrom.
	DateFrom *time.Time
	// DateUntil, when set, keeps only messages created at or before *DateUntil.
	DateUntil *time.Time
}

// Matches reports whether the given message satisfies the filter's criteria.
//
// It is used for live, in-memory filtering by the WebSocket stream and plugin
// delivery; the persisted REST path applies the same criteria as SQL instead.
// A nil receiver matches everything.
func (f *MessageFilter) Matches(msg *MessageExternal) bool {
	if f == nil || msg == nil {
		return true
	}

	if len(f.AppIDs) > 0 {
		found := false
		for _, id := range f.AppIDs {
			if id == msg.ApplicationID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	priority := 0
	if msg.Priority != nil {
		priority = *msg.Priority
	}
	if f.PriorityFrom != nil && priority < *f.PriorityFrom {
		return false
	}
	if f.PriorityUntil != nil && priority > *f.PriorityUntil {
		return false
	}

	if f.DateFrom != nil && msg.Date.Before(*f.DateFrom) {
		return false
	}
	if f.DateUntil != nil && msg.Date.After(*f.DateUntil) {
		return false
	}

	return true
}

// AddToQuery writes the filter's set criteria back into the given query values.
//
// Only populated criteria are written, so an empty filter adds nothing. This is
// used to carry the active filter forward into the "next" pagination link. A nil
// receiver is a no-op.
func (f *MessageFilter) AddToQuery(values url.Values) {
	if f == nil {
		return
	}
	for _, id := range f.AppIDs {
		values.Add("appid", strconv.FormatUint(uint64(id), 10))
	}
	if f.PriorityFrom != nil {
		values.Add("priorityFrom", strconv.Itoa(*f.PriorityFrom))
	}
	if f.PriorityUntil != nil {
		values.Add("priorityUntil", strconv.Itoa(*f.PriorityUntil))
	}
	if f.DateFrom != nil {
		values.Add("dateFrom", f.DateFrom.Format(time.RFC3339))
	}
	if f.DateUntil != nil {
		values.Add("dateUntil", f.DateUntil.Format(time.RFC3339))
	}
}

// ParseMessageFilter builds a MessageFilter from query values. It is shared by
// the REST handlers and the WebSocket stream so both interpret the same query
// parameters identically.
//
// Recognized parameters: appid (repeatable), priorityFrom, priorityUntil,
// dateFrom, dateUntil (RFC3339). Empty appid values are ignored. The returned
// filter is never nil; an error (which callers map to HTTP 400) is returned for
// malformed values.
func ParseMessageFilter(values url.Values) (*MessageFilter, error) {
	filter := &MessageFilter{}

	for _, raw := range values["appid"] {
		if raw == "" {
			continue
		}
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid appid %q", raw)
		}
		filter.AppIDs = append(filter.AppIDs, uint(id))
	}

	var err error
	if filter.PriorityFrom, err = parseOptionalInt(values.Get("priorityFrom")); err != nil {
		return nil, fmt.Errorf("invalid priorityFrom: %w", err)
	}
	if filter.PriorityUntil, err = parseOptionalInt(values.Get("priorityUntil")); err != nil {
		return nil, fmt.Errorf("invalid priorityUntil: %w", err)
	}
	if filter.DateFrom, err = parseOptionalTime(values.Get("dateFrom")); err != nil {
		return nil, fmt.Errorf("invalid dateFrom: %w", err)
	}
	if filter.DateUntil, err = parseOptionalTime(values.Get("dateUntil")); err != nil {
		return nil, fmt.Errorf("invalid dateUntil: %w", err)
	}

	return filter, nil
}

func parseOptionalInt(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
