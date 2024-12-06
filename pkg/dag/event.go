package dag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Event represents a node in the DAG
type Event struct {
	ID        string    `json:"id"`
	Data      []byte    `json:"data"`
	Parents   []string  `json:"parents"` // References to previous events in DAG
	Timestamp time.Time `json:"timestamp"`

	// Optional beacon reference
	BeaconRound uint64 `json:"beacon_round,omitempty"`
	BeaconHash  string `json:"beacon_hash,omitempty"`

	// Sub-events support
	ParentEvent string   `json:"parent_event,omitempty"` // ID of the parent event if this is a sub-event
	SubEvents   []string `json:"sub_events,omitempty"`   // IDs of any sub-events
	IsSubEvent  bool     `json:"is_sub_event,omitempty"` // Flag to distinguish sub-events
}

// NewEvent creates a new event with the given data and parents
func NewEvent(data []byte, parents []string) (*Event, error) {
	event := &Event{
		Data:      data,
		Parents:   parents,
		Timestamp: time.Now(),
	}

	// Calculate event ID
	if err := event.calculateID(); err != nil {
		return nil, err
	}

	return event, nil
}

// NewSubEvent creates a new sub-event connected to a parent event
func NewSubEvent(data []byte, parentEventID string, additionalParents []string) (*Event, error) {
	// Combine parent event ID with any additional parents
	parents := append([]string{parentEventID}, additionalParents...)

	event := &Event{
		Data:        data,
		Parents:     parents,
		Timestamp:   time.Now(),
		ParentEvent: parentEventID,
		IsSubEvent:  true,
	}

	// Calculate event ID
	if err := event.calculateID(); err != nil {
		return nil, err
	}

	return event, nil
}

// AddSubEvent adds a sub-event to this event
func (e *Event) AddSubEvent(subEventID string) {
	if e.SubEvents == nil {
		e.SubEvents = make([]string, 0)
	}
	e.SubEvents = append(e.SubEvents, subEventID)
}

// SetBeaconRound sets the beacon round reference for this event
func (e *Event) SetBeaconRound(round uint64, randomness []byte) {
	e.BeaconRound = round
	e.BeaconHash = hex.EncodeToString(randomness)
}

// calculateID generates a unique ID for the event based on its contents
func (e *Event) calculateID() error {
	// Create a map of all fields to hash
	hashInput := struct {
		Data        []byte    `json:"data"`
		Parents     []string  `json:"parents"`
		Timestamp   time.Time `json:"timestamp"`
		ParentEvent string    `json:"parent_event,omitempty"`
		IsSubEvent  bool      `json:"is_sub_event,omitempty"`
	}{
		Data:        e.Data,
		Parents:     e.Parents,
		Timestamp:   e.Timestamp,
		ParentEvent: e.ParentEvent,
		IsSubEvent:  e.IsSubEvent,
	}

	// Marshal to JSON for consistent hashing
	data, err := json.Marshal(hashInput)
	if err != nil {
		return err
	}

	// Calculate SHA-256 hash
	hash := sha256.Sum256(data)
	e.ID = hex.EncodeToString(hash[:])

	return nil
}
