package dag

import (
	"fmt"
	"sync"
)

// DAG represents a directed acyclic graph of events
type DAG struct {
	mu     sync.RWMutex
	events map[string]*Event // Map of event ID to event
}

// New creates a new empty DAG
func New() *DAG {
	return &DAG{
		events: make(map[string]*Event),
	}
}

// AddEvent adds a new event to the DAG
func (d *DAG) AddEvent(event *Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verify event has an ID
	if event.ID == "" {
		return fmt.Errorf("event has no ID")
	}

	// Check if event already exists
	if _, exists := d.events[event.ID]; exists {
		return fmt.Errorf("event %s already exists", event.ID)
	}

	// Verify all parent events exist
	for _, parentID := range event.Parents {
		if _, exists := d.events[parentID]; !exists {
			return fmt.Errorf("parent event %s does not exist", parentID)
		}
	}

	// If this is a sub-event, verify parent event exists
	if event.ParentEvent != "" {
		if _, exists := d.events[event.ParentEvent]; !exists {
			return fmt.Errorf("parent event %s does not exist", event.ParentEvent)
		}
	}

	// Only check for cycles if this is not a sub-event
	if !event.IsSubEvent {
		if err := d.wouldCreateCycle(event); err != nil {
			return fmt.Errorf("adding event would create cycle: %w", err)
		}
	}

	// Add the event
	d.events[event.ID] = event

	// Update parent event's sub-events list if this is a sub-event
	if event.ParentEvent != "" {
		parentEvent := d.events[event.ParentEvent]
		parentEvent.AddSubEvent(event.ID)
	}

	return nil
}

// wouldCreateCycle checks if adding the event would create a cycle
func (d *DAG) wouldCreateCycle(newEvent *Event) error {
	// For each parent, check if we can reach any other parent by following parent links
	for i, startParentID := range newEvent.Parents {
		visited := make(map[string]bool)

		var canReach func(currentID string, targetID string) bool
		canReach = func(currentID string, targetID string) bool {
			if currentID == targetID {
				return true
			}

			if visited[currentID] {
				return false
			}
			visited[currentID] = true

			// Get the current event
			currentEvent := d.events[currentID]
			if currentEvent == nil {
				return false
			}

			// Only follow non-sub-event parent relationships
			for _, parentID := range currentEvent.Parents {
				parentEvent := d.events[parentID]
				if parentEvent == nil || parentEvent.IsSubEvent {
					continue
				}

				if canReach(parentID, targetID) {
					return true
				}
			}

			return false
		}

		// Check if we can reach any other parent from this one
		for j, endParentID := range newEvent.Parents {
			if i != j {
				// Reset visited map for each check
				visited = make(map[string]bool)

				if canReach(startParentID, endParentID) {
					return fmt.Errorf("cycle detected: can reach parent %s from parent %s", endParentID, startParentID)
				}
			}
		}
	}

	return nil
}

// GetEvent retrieves an event by its ID
func (d *DAG) GetEvent(id string) (*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, fmt.Errorf("event %s not found", id)
	}

	return event, nil
}

// GetSubEvents retrieves all sub-events for a given event
func (d *DAG) GetSubEvents(eventID string) ([]*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[eventID]
	if !exists {
		return nil, fmt.Errorf("event %s not found", eventID)
	}

	subEvents := make([]*Event, 0, len(event.SubEvents))
	for _, subEventID := range event.SubEvents {
		if subEvent, exists := d.events[subEventID]; exists {
			subEvents = append(subEvents, subEvent)
		}
	}

	return subEvents, nil
}

// GetParents returns all direct parent events of the given event
func (d *DAG) GetParents(eventID string) ([]*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[eventID]
	if !exists {
		return nil, fmt.Errorf("event %s not found", eventID)
	}

	parents := make([]*Event, 0, len(event.Parents))
	for _, parentID := range event.Parents {
		if parent, exists := d.events[parentID]; exists {
			parents = append(parents, parent)
		}
	}

	return parents, nil
}

// GetEvents returns all events in the DAG
func (d *DAG) GetEvents() []*Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	events := make([]*Event, 0, len(d.events))
	for _, event := range d.events {
		events = append(events, event)
	}

	return events
}

// GetEventsWithBeacon returns all events that reference a specific beacon round
func (d *DAG) GetEventsWithBeacon(round uint64) []*Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var events []*Event
	for _, event := range d.events {
		if event.BeaconRound == round {
			events = append(events, event)
		}
	}

	return events
}
