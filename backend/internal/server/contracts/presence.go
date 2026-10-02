package contracts

import "time"

const PresenceKey = "hosts.presence"

type HostPresence struct {
	HostID      string    `json:"hostId"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	IdleSeconds *int64    `json:"idleSeconds,omitempty"`
	Locked      *bool     `json:"locked,omitempty"`
	DisplayOff  *bool     `json:"displayOff,omitempty"`
	Known       bool      `json:"known"`
	Online      bool      `json:"online"`
	Since       time.Time `json:"since"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Presence interface {
	State(hostID string) HostPresence
	Subscribe() (<-chan HostPresence, func())
}
