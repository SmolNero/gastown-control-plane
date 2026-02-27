package model

import (
	"encoding/json"
	"time"
)

type Event struct {
	SchemaVersion int             `json:"schema_version,omitempty"`
	Type          string          `json:"type"`
	Source        string          `json:"source,omitempty"`
	Rig           string          `json:"rig,omitempty"`
	Agent         string          `json:"agent,omitempty"`
	Convoy        string          `json:"convoy,omitempty"`
	Hook          string          `json:"hook,omitempty"`
	Status        string          `json:"status,omitempty"`
	Message       string          `json:"message,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at,omitempty"`
}

type Snapshot struct {
	SchemaVersion int       `json:"schema_version,omitempty"`
	Source        string    `json:"source,omitempty"`
	Workspace     string    `json:"workspace,omitempty"`
	Host          string    `json:"host,omitempty"`
	CollectedAt   time.Time `json:"collected_at"`
	Rigs          []Rig     `json:"rigs,omitempty"`
	Agents        []Agent   `json:"agents,omitempty"`
	Convoys       []Convoy  `json:"convoys,omitempty"`
	Hooks         []Hook    `json:"hooks,omitempty"`
}

type Rig struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type Agent struct {
	Name       string          `json:"name"`
	Rig        string          `json:"rig,omitempty"`
	Role       string          `json:"role,omitempty"`
	Status     string          `json:"status,omitempty"`
	LastSeenAt time.Time       `json:"last_seen_at,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type Convoy struct {
	Name   string `json:"name"`
	Rig    string `json:"rig,omitempty"`
	Status string `json:"status,omitempty"`
}

type Hook struct {
	Name   string `json:"name"`
	Rig    string `json:"rig,omitempty"`
	Status string `json:"status,omitempty"`
}

type Alert struct {
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Message    string    `json:"message"`
	Rig        string    `json:"rig,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
}

type APIKey struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
}
