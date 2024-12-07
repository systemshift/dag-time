package dag

import (
	"fmt"
	"sync"
)

// DAG manages the directed acyclic graph of events
type DAG struct {
	mu     sync.RWMutex
	events map[string]*Event
}

// NewDAG creates a new DAG instance
func NewDAG() *DAG {
	return &DAG{
		events: make(map[string]*Event),
	}
}

// AddEvent adds a new event to the DAG
func (d *DAG) AddEvent(event *Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()

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
	if event.IsSubEvent {
		if _, exists := d.events[event.ParentEvent]; !exists {
			return fmt.Errorf("parent event %s for sub-event does not exist", event.ParentEvent)
		}
	}

	// Verify adding this event won't create a cycle
	if err := d.detectCycle(event); err != nil {
		return fmt.Errorf("adding event would create cycle: %w", err)
	}

	// Add event to DAG
	d.events[event.ID] = event

	// If this is a sub-event, update the parent event's sub-events list
	if event.IsSubEvent {
		parentEvent := d.events[event.ParentEvent]
		parentEvent.AddSubEvent(event.ID)
	}

	return nil
}

// detectCycle checks if adding the new event would create a cycle
func (d *DAG) detectCycle(newEvent *Event) error {
	visited := make(map[string]bool)
	path := make(map[string]bool)

	var visit func(eventID string, isSubEventPath bool) error
	visit = func(eventID string, isSubEventPath bool) error {
		if path[eventID] {
			return fmt.Errorf("cycle detected at event %s", eventID)
		}
		if visited[eventID] {
			return nil
		}

		visited[eventID] = true
		path[eventID] = true

		event := d.events[eventID]
		if event != nil {
			// Check parent relationships
			for _, parentID := range event.Parents {
				if err := visit(parentID, false); err != nil {
					return err
				}
			}

			// Only follow sub-event relationships if we're not already on a sub-event path
			if !isSubEventPath {
				for _, subEventID := range event.SubEvents {
					if err := visit(subEventID, true); err != nil {
						return err
					}
				}
			}
		}

		path[eventID] = false
		return nil
	}

	// Check paths from new event's parents
	for _, parentID := range newEvent.Parents {
		if err := visit(parentID, newEvent.IsSubEvent); err != nil {
			return err
		}
	}

	return nil
}

// GetEvent retrieves an event by ID
func (d *DAG) GetEvent(id string) (*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, fmt.Errorf("event %s not found", id)
	}
	return event, nil
}

// GetAllEvents returns all events in the DAG
func (d *DAG) GetAllEvents() []*Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	events := make([]*Event, 0, len(d.events))
	for _, event := range d.events {
		events = append(events, event)
	}
	return events
}

// VerifyEventChain verifies the integrity of an event and its ancestors
func (d *DAG) VerifyEventChain(eventID string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[string]bool)

	var verify func(id string) error
	verify = func(id string) error {
		if visited[id] {
			return nil
		}

		event, exists := d.events[id]
		if !exists {
			return fmt.Errorf("event %s not found", id)
		}

		// Verify parent relationships
		for _, parentID := range event.Parents {
			parent, exists := d.events[parentID]
			if !exists {
				return fmt.Errorf("parent event %s not found", parentID)
			}

			// Verify temporal ordering
			if !parent.Timestamp.Before(event.Timestamp) {
				return fmt.Errorf("invalid temporal ordering between %s and parent %s", id, parentID)
			}

			if err := verify(parentID); err != nil {
				return err
			}
		}

		// If this is a sub-event, verify relationship with parent event
		if event.IsSubEvent {
			parentEvent, exists := d.events[event.ParentEvent]
			if !exists {
				return fmt.Errorf("parent event %s not found", event.ParentEvent)
			}

			// Verify temporal ordering
			if !parentEvent.Timestamp.Before(event.Timestamp) {
				return fmt.Errorf("invalid temporal ordering between sub-event %s and parent %s", id, event.ParentEvent)
			}

			// Verify parent event has this sub-event in its list
			found := false
			for _, subEventID := range parentEvent.SubEvents {
				if subEventID == id {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parent event %s does not reference sub-event %s", event.ParentEvent, id)
			}
		}

		visited[id] = true
		return nil
	}

	return verify(eventID)
}
