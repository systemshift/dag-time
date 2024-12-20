package dag

import (
	"testing"
)

func TestDAG_AddEvent(t *testing.T) {
	dag := New()

	// Create and add a root event
	root, err := NewEvent([]byte("root"), nil)
	if err != nil {
		t.Fatalf("Failed to create root event: %v", err)
	}

	if err := dag.AddEvent(root); err != nil {
		t.Fatalf("Failed to add root event: %v", err)
	}

	// Create and add a child event
	child, err := NewEvent([]byte("child"), []string{root.ID})
	if err != nil {
		t.Fatalf("Failed to create child event: %v", err)
	}

	if err := dag.AddEvent(child); err != nil {
		t.Fatalf("Failed to add child event: %v", err)
	}

	// Verify events were added
	if len(dag.GetEvents()) != 2 {
		t.Errorf("Expected 2 events, got %d", len(dag.GetEvents()))
	}

	// Try to add duplicate event
	if err := dag.AddEvent(root); err == nil {
		t.Error("Expected error when adding duplicate event")
	}

	// Try to add event with non-existent parent
	invalid, err := NewEvent([]byte("invalid"), []string{"nonexistent"})
	if err != nil {
		t.Fatalf("Failed to create invalid event: %v", err)
	}

	if err := dag.AddEvent(invalid); err == nil {
		t.Error("Expected error when adding event with non-existent parent")
	}
}

func TestDAG_SubEvents(t *testing.T) {
	dag := New()

	// Create parent event
	parent, err := NewEvent([]byte("parent"), nil)
	if err != nil {
		t.Fatalf("Failed to create parent event: %v", err)
	}

	if err := dag.AddEvent(parent); err != nil {
		t.Fatalf("Failed to add parent event: %v", err)
	}

	// Create sub-event
	subEvent, err := NewSubEvent([]byte("sub-event"), parent.ID, nil)
	if err != nil {
		t.Fatalf("Failed to create sub-event: %v", err)
	}

	if err := dag.AddEvent(subEvent); err != nil {
		t.Fatalf("Failed to add sub-event: %v", err)
	}

	// Verify sub-event relationship
	subEvents, err := dag.GetSubEvents(parent.ID)
	if err != nil {
		t.Fatalf("Failed to get sub-events: %v", err)
	}

	if len(subEvents) != 1 {
		t.Errorf("Expected 1 sub-event, got %d", len(subEvents))
	}

	if subEvents[0].ID != subEvent.ID {
		t.Errorf("Expected sub-event ID %s, got %s", subEvent.ID, subEvents[0].ID)
	}

	// Create another sub-event with additional parent
	subEvent2, err := NewSubEvent([]byte("sub-event-2"), parent.ID, []string{subEvent.ID})
	if err != nil {
		t.Fatalf("Failed to create second sub-event: %v", err)
	}

	if err := dag.AddEvent(subEvent2); err != nil {
		t.Fatalf("Failed to add second sub-event: %v", err)
	}

	// Verify both sub-events are present
	subEvents, err = dag.GetSubEvents(parent.ID)
	if err != nil {
		t.Fatalf("Failed to get sub-events: %v", err)
	}

	if len(subEvents) != 2 {
		t.Errorf("Expected 2 sub-events, got %d", len(subEvents))
	}
}

func TestDAG_CycleDetection(t *testing.T) {
	dag := New()

	// Create events
	event1, _ := NewEvent([]byte("event1"), nil)
	event2, _ := NewEvent([]byte("event2"), []string{event1.ID})
	event3, _ := NewEvent([]byte("event3"), []string{event2.ID})

	// Add events
	if err := dag.AddEvent(event1); err != nil {
		t.Fatalf("Failed to add event1: %v", err)
	}
	if err := dag.AddEvent(event2); err != nil {
		t.Fatalf("Failed to add event2: %v", err)
	}
	if err := dag.AddEvent(event3); err != nil {
		t.Fatalf("Failed to add event3: %v", err)
	}

	// Try to create a cycle
	cyclicEvent, _ := NewEvent([]byte("cyclic"), []string{event3.ID, event1.ID})
	if err := dag.AddEvent(cyclicEvent); err == nil {
		t.Error("Expected error when creating cycle")
	}
}

func TestDAG_BeaconReference(t *testing.T) {
	dag := New()

	// Create event with beacon reference
	event, err := NewEvent([]byte("event"), nil)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	// Set beacon round
	event.SetBeaconRound(123, []byte("random"))

	if err := dag.AddEvent(event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	// Get events with beacon round
	events := dag.GetEventsWithBeacon(123)
	if len(events) != 1 {
		t.Errorf("Expected 1 event with beacon round 123, got %d", len(events))
	}

	if events[0].BeaconRound != 123 {
		t.Errorf("Expected beacon round 123, got %d", events[0].BeaconRound)
	}
}

func TestDAG_GetParents(t *testing.T) {
	dag := New()

	// Create events
	parent1, _ := NewEvent([]byte("parent1"), nil)
	parent2, _ := NewEvent([]byte("parent2"), nil)
	child, _ := NewEvent([]byte("child"), []string{parent1.ID, parent2.ID})

	// Add events
	if err := dag.AddEvent(parent1); err != nil {
		t.Fatalf("Failed to add parent1: %v", err)
	}
	if err := dag.AddEvent(parent2); err != nil {
		t.Fatalf("Failed to add parent2: %v", err)
	}
	if err := dag.AddEvent(child); err != nil {
		t.Fatalf("Failed to add child: %v", err)
	}

	// Get parents
	parents, err := dag.GetParents(child.ID)
	if err != nil {
		t.Fatalf("Failed to get parents: %v", err)
	}

	if len(parents) != 2 {
		t.Errorf("Expected 2 parents, got %d", len(parents))
	}

	// Verify parent IDs
	parentIDs := make(map[string]bool)
	for _, parent := range parents {
		parentIDs[parent.ID] = true
	}

	if !parentIDs[parent1.ID] || !parentIDs[parent2.ID] {
		t.Error("Missing expected parent IDs")
	}
}
