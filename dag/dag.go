// Package dag implements a directed acyclic graph for event ordering
package dag

import (
	"context"
	"fmt"
)

// Common errors
var (
	ErrNotFound       = fmt.Errorf("event not found")
	ErrDuplicateEvent = fmt.Errorf("event already exists")
	ErrInvalidEvent   = fmt.Errorf("invalid event")
	ErrMissingParent  = fmt.Errorf("parent event not found")
	ErrCycleDetected  = fmt.Errorf("cycle detected in DAG")
)

// EventType indicates whether an event is a main event or sub-event
type EventType int

const (
	// MainEvent represents a top-level event in the DAG
	MainEvent EventType = iota
	// SubEvent represents a sub-event that can have complex relationships
	SubEvent
)

// Event represents a node in the DAG
type Event struct {
	// ID uniquely identifies the event
	ID string

	// Type indicates if this is a main event or sub-event
	Type EventType

	// ParentID is the ID of the main event that spawned this event
	// Empty for main events
	ParentID string

	// Parents are the direct predecessors of this event
	// For main events, these are other main events
	// For sub-events, these can be:
	// - The main event that spawned it
	// - Other sub-events from the same parent
	// - Sub-events from different parents (cross-event relationships)
	Parents []string

	// Children are the IDs of events that reference this event
	// This enables traversing sub-event relationships in both directions
	Children []string

	// Data is the event payload
	Data []byte

	// Beacon is the optional anchor to a drand beacon round
	Beacon *BeaconAnchor
}

// BeaconAnchor represents an anchor to a drand beacon round
type BeaconAnchor struct {
	Round      uint64
	Randomness []byte
}

// DAG represents a directed acyclic graph of events
type DAG interface {
	// AddEvent adds a new event to the DAG
	// For sub-events, Parents can include:
	// - The main event that spawned it (required for sub-events)
	// - Other sub-events from the same parent
	// - Sub-events from different parents
	AddEvent(ctx context.Context, event *Event) error

	// GetEvent retrieves an event by ID
	GetEvent(ctx context.Context, id string) (*Event, error)

	// GetParents returns the parent events of the given event
	GetParents(ctx context.Context, id string) ([]*Event, error)

	// GetChildren returns the child events of the given event
	// This includes both direct sub-events and events that reference it
	GetChildren(ctx context.Context, id string) ([]*Event, error)

	// GetSubEvents returns all sub-events spawned by the given event
	// This includes the entire sub-event network, not just direct children
	GetSubEvents(ctx context.Context, id string) ([]*Event, error)

	// GetMainEvents returns all main events in the DAG
	// Optionally filtered by a time range using beacon timestamps
	GetMainEvents(ctx context.Context) ([]*Event, error)

	// Verify checks the integrity of the DAG including:
	// - No cycles in event relationships
	// - Valid parent-child relationships
	// - Sub-events have valid parent events
	// - Beacon anchors are properly ordered
	Verify(ctx context.Context) error
}
