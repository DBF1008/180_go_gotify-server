package model

import "time"

// MessageFilter defines structured filtering criteria for messages.
// All fields are optional; nil values mean "no filter" for that criterion.
// This struct is the single source of truth for filtering and is reused
// by the REST API (query binding), WebSocket (subscription config), and Plugins.
type MessageFilter struct {
	// ApplicationID filters by a specific application.
	ApplicationID *uint `form:"appid" json:"appid,omitempty"`

	// Priority filters for an exact priority match.
	Priority *int `form:"priority" json:"priority,omitempty"`

	// PriorityMin filters for messages with priority >= this value.
	PriorityMin *int `form:"priority_min" json:"priority_min,omitempty"`

	// PriorityMax filters for messages with priority <= this value.
	PriorityMax *int `form:"priority_max" json:"priority_max,omitempty"`

	// Before filters for messages with date strictly before this time.
	Before *time.Time `form:"before" json:"before,omitempty"`

	// After filters for messages with date strictly after this time.
	After *time.Time `form:"after" json:"after,omitempty"`
}

// IsActive returns true if any filter criterion is set.
func (f *MessageFilter) IsActive() bool {
	if f == nil {
		return false
	}
	return f.ApplicationID != nil || f.Priority != nil ||
		f.PriorityMin != nil || f.PriorityMax != nil ||
		f.Before != nil || f.After != nil
}

// MatchesMessage checks if a MessageExternal passes this filter.
// A nil filter matches everything.
func (f *MessageFilter) MatchesMessage(msg *MessageExternal) bool {
	if f == nil {
		return true
	}
	if f.ApplicationID != nil && msg.ApplicationID != *f.ApplicationID {
		return false
	}
	if f.Priority != nil && (msg.Priority == nil || *msg.Priority != *f.Priority) {
		return false
	}
	if f.PriorityMin != nil && (msg.Priority == nil || *msg.Priority < *f.PriorityMin) {
		return false
	}
	if f.PriorityMax != nil && (msg.Priority == nil || *msg.Priority > *f.PriorityMax) {
		return false
	}
	if f.Before != nil && !msg.Date.Before(*f.Before) {
		return false
	}
	if f.After != nil && !msg.Date.After(*f.After) {
		return false
	}
	return true
}
