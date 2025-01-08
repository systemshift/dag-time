package dag

import (
	"context"
	"fmt"
	"sync"
)

// memoryDAG implements the DAG interface with an in-memory storage
type memoryDAG struct {
	mu     sync.RWMutex
	events map[string]*Event
}

// NewMemoryDAG creates a new DAG with in-memory storage
func NewMemoryDAG() DAG {
	return &memoryDAG{
		events: make(map[string]*Event),
	}
}

func (d *memoryDAG) AddEvent(ctx context.Context, event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}
	if event.ID == "" {
		return fmt.Errorf("event ID cannot be empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if event already exists
	if _, exists := d.events[event.ID]; exists {
		return fmt.Errorf("event %s already exists", event.ID)
	}

	// Verify all parents exist
	for _, parentID := range event.Parents {
		if _, exists := d.events[parentID]; !exists {
			return fmt.Errorf("parent event %s does not exist", parentID)
		}
	}

	// Store the event
	d.events[event.ID] = event
	return nil
}

func (d *memoryDAG) GetEvent(ctx context.Context, id string) (*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, fmt.Errorf("event %s not found", id)
	}

	return event, nil
}

func (d *memoryDAG) GetParents(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, fmt.Errorf("event %s not found", id)
	}

	parents := make([]*Event, 0, len(event.Parents))
	for _, parentID := range event.Parents {
		parent, exists := d.events[parentID]
		if !exists {
			return nil, fmt.Errorf("parent event %s not found", parentID)
		}
		parents = append(parents, parent)
	}

	return parents, nil
}

func (d *memoryDAG) Verify(ctx context.Context) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check each event's parents exist
	for id, event := range d.events {
		for _, parentID := range event.Parents {
			if _, exists := d.events[parentID]; !exists {
				return fmt.Errorf("event %s references non-existent parent %s", id, parentID)
			}
		}
	}

	// Check for cycles using depth-first search
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var checkCycle func(string) error
	checkCycle = func(id string) error {
		visited[id] = true
		inStack[id] = true
		defer func() { inStack[id] = false }()

		event := d.events[id]
		for _, parentID := range event.Parents {
			if !visited[parentID] {
				if err := checkCycle(parentID); err != nil {
					return err
				}
			} else if inStack[parentID] {
				return fmt.Errorf("cycle detected involving events %s and %s", id, parentID)
			}
		}

		return nil
	}

	for id := range d.events {
		if !visited[id] {
			if err := checkCycle(id); err != nil {
				return err
			}
		}
	}

	return nil
}
