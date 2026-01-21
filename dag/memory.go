package dag

import (
	"context"
	"fmt"
	"io"
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
		return fmt.Errorf("%w: event cannot be nil", ErrInvalidEvent)
	}
	if event.ID == "" {
		return fmt.Errorf("%w: event ID cannot be empty", ErrInvalidEvent)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if event already exists
	if _, exists := d.events[event.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateEvent, event.ID)
	}

	// For sub-events, verify parent exists
	if event.Type == SubEvent {
		if event.ParentID == "" {
			return fmt.Errorf("%w: sub-event must have a parent ID", ErrInvalidEvent)
		}
		parent, exists := d.events[event.ParentID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrMissingParent, event.ParentID)
		}
		// Add this event to parent's children
		parent.Children = append(parent.Children, event.ID)
	}

	// Verify all parents exist
	for _, parentID := range event.Parents {
		if _, exists := d.events[parentID]; !exists {
			return fmt.Errorf("%w: %s", ErrMissingParent, parentID)
		}
		// Add this event to each parent's children
		d.events[parentID].Children = append(d.events[parentID].Children, event.ID)
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
		return nil, ErrNotFound
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
		return nil, ErrNotFound
	}

	parents := make([]*Event, 0, len(event.Parents))
	for _, parentID := range event.Parents {
		parent, exists := d.events[parentID]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrMissingParent, parentID)
		}
		parents = append(parents, parent)
	}

	return parents, nil
}

func (d *memoryDAG) GetChildren(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, ErrNotFound
	}

	children := make([]*Event, 0, len(event.Children))
	for _, childID := range event.Children {
		child, exists := d.events[childID]
		if !exists {
			return nil, fmt.Errorf("%w: child %s", ErrNotFound, childID)
		}
		children = append(children, child)
	}

	return children, nil
}

func (d *memoryDAG) GetSubEvents(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.events[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Use a map to track visited events and avoid duplicates
	visited := make(map[string]bool)
	subEvents := make([]*Event, 0)

	// Helper function for recursive traversal
	var traverse func(eventID string) error
	traverse = func(eventID string) error {
		if visited[eventID] {
			return nil
		}
		visited[eventID] = true

		event, exists := d.events[eventID]
		if !exists {
			return fmt.Errorf("event %s not found during traversal", eventID)
		}

		if event.Type == SubEvent && event.ParentID == id {
			subEvents = append(subEvents, event)
			// Traverse this sub-event's children
			for _, childID := range event.Children {
				if err := traverse(childID); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Start with direct children
	for _, childID := range event.Children {
		if err := traverse(childID); err != nil {
			return nil, err
		}
	}

	return subEvents, nil
}

func (d *memoryDAG) GetMainEvents(ctx context.Context) ([]*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	mainEvents := make([]*Event, 0)
	for _, event := range d.events {
		if event.Type == MainEvent {
			mainEvents = append(mainEvents, event)
		}
	}

	return mainEvents, nil
}

func (d *memoryDAG) Verify(ctx context.Context) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check each event's references are valid
	for id, event := range d.events {
		// Check parent event exists for sub-events
		if event.Type == SubEvent {
			if event.ParentID == "" {
				return fmt.Errorf("sub-event %s has no parent ID", id)
			}
			if _, exists := d.events[event.ParentID]; !exists {
				return fmt.Errorf("sub-event %s references non-existent parent %s", id, event.ParentID)
			}
		}

		// Check all parents exist
		for _, parentID := range event.Parents {
			if _, exists := d.events[parentID]; !exists {
				return fmt.Errorf("event %s references non-existent parent %s", id, parentID)
			}
		}

		// Check all children exist
		for _, childID := range event.Children {
			if _, exists := d.events[childID]; !exists {
				return fmt.Errorf("event %s references non-existent child %s", id, childID)
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
				return fmt.Errorf("%w: involving events %s and %s", ErrCycleDetected, id, parentID)
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

// Export writes the DAG state to the provided writer
func (d *memoryDAG) Export(ctx context.Context, w io.Writer) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Build children map from events
	children := make(map[string][]string)
	for _, event := range d.events {
		if len(event.Children) > 0 {
			children[event.ID] = event.Children
		}
	}

	return ExportDAG(ctx, w, d.events, children)
}

// Import reads the DAG state from the provided reader
func (d *memoryDAG) Import(ctx context.Context, r io.Reader) error {
	events, children, err := ImportDAG(ctx, r)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Restore events
	d.events = events

	// Restore children references
	for parentID, childIDs := range children {
		if event, exists := d.events[parentID]; exists {
			event.Children = childIDs
		}
	}

	return nil
}
