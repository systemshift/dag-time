// Package dag implements a directed acyclic graph for event ordering
package dag

import (
	"context"
)

// Event represents a node in the DAG
type Event struct {
	ID      string
	Parents []string
	Data    []byte
	Beacon  *BeaconAnchor
}

// BeaconAnchor represents an anchor to a drand beacon round
type BeaconAnchor struct {
	Round      uint64
	Randomness []byte
}

// DAG represents a directed acyclic graph of events
type DAG interface {
	// AddEvent adds a new event to the DAG
	AddEvent(ctx context.Context, event *Event) error

	// GetEvent retrieves an event by ID
	GetEvent(ctx context.Context, id string) (*Event, error)

	// GetParents returns the parent events of the given event
	GetParents(ctx context.Context, id string) ([]*Event, error)

	// Verify checks the integrity of the DAG
	Verify(ctx context.Context) error
}
