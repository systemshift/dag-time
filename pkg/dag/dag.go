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
	// Helper function to check if eventB is reachable from eventA through parent relationships
	var isReachable func(eventA, eventB string, visited map[string]bool, checkSubEvents bool) bool
	isReachable = func(eventA, eventB string, visited map[string]bool, checkSubEvents bool) bool {
		if eventA == eventB {
			return true
		}

		if visited[eventA] {
			return false
		}
		visited[eventA] = true

		event := d.events[eventA]
		if event == nil {
			return false
		}

		// Follow regular parent relationships
		for _, parentID := range event.Parents {
			if isReachable(parentID, eventB, visited, checkSubEvents) {
				return true
			}
		}

		// Only check sub-event relationships if requested
		if checkSubEvents && event.IsSubEvent {
			if isReachable(event.ParentEvent, eventB, visited, true) {
				return true
			}
		}

		return false
	}

	// Add the new event temporarily to check for cycles
	tempEvent := *newEvent
	d.events[tempEvent.ID] = &tempEvent
	defer delete(d.events, tempEvent.ID)

	if tempEvent.IsSubEvent {
		// For sub-events, only check cycles through the parent event
		visited := make(map[string]bool)
		if isReachable(tempEvent.ParentEvent, tempEvent.ID, visited, false) {
			return fmt.Errorf("cycle detected through parent event %s", tempEvent.ParentEvent)
		}
	} else {
		// For regular events, check for cycles through parent relationships
		for _, parent1 := range tempEvent.Parents {
			// Check if this parent can reach any other parent
			for _, parent2 := range tempEvent.Parents {
				if parent1 == parent2 {
					continue
				}
				visited := make(map[string]bool)
				if isReachable(parent1, parent2, visited, false) {
					return fmt.Errorf("cycle detected: parent %s can reach parent %s", parent1, parent2)
				}
			}

			// Check if this parent can reach the new event through other paths
			visited := make(map[string]bool)
			delete(d.events, tempEvent.ID) // Temporarily remove the event to check existing paths
			if isReachable(parent1, tempEvent.ID, visited, false) {
				d.events[tempEvent.ID] = &tempEvent // Restore the event
				return fmt.Errorf("cycle detected: parent %s can reach event through existing path", parent1)
			}
			d.events[tempEvent.ID] = &tempEvent // Restore the event
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
