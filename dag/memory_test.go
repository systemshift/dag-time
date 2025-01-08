package dag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryDAG(t *testing.T) {
	ctx := context.Background()

	t.Run("AddEvent", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Test adding nil event
		err := dag.AddEvent(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")

		// Test adding event with empty ID
		err = dag.AddEvent(ctx, &Event{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event ID cannot be empty")

		// Test adding valid main event
		event := &Event{
			ID:   "event1",
			Type: MainEvent,
			Data: []byte("test"),
		}
		err = dag.AddEvent(ctx, event)
		assert.NoError(t, err)

		// Test adding duplicate event
		err = dag.AddEvent(ctx, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// Test adding sub-event without parent ID
		subEvent := &Event{
			ID:   "sub1",
			Type: SubEvent,
			Data: []byte("sub-event"),
		}
		err = dag.AddEvent(ctx, subEvent)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must have a parent ID")

		// Test adding valid sub-event
		subEvent.ParentID = "event1"
		err = dag.AddEvent(ctx, subEvent)
		assert.NoError(t, err)

		// Verify parent's children list was updated
		parent, err := dag.GetEvent(ctx, "event1")
		assert.NoError(t, err)
		assert.Contains(t, parent.Children, "sub1")
	})

	t.Run("Complex Event Relationships", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Create main events
		main1 := &Event{ID: "main1", Type: MainEvent}
		main2 := &Event{ID: "main2", Type: MainEvent}
		require.NoError(t, dag.AddEvent(ctx, main1))
		require.NoError(t, dag.AddEvent(ctx, main2))

		// Create sub-events for main1
		sub1a := &Event{
			ID:       "sub1a",
			Type:     SubEvent,
			ParentID: "main1",
		}
		sub1b := &Event{
			ID:       "sub1b",
			Type:     SubEvent,
			ParentID: "main1",
			Parents:  []string{"sub1a"}, // Reference another sub-event
		}
		require.NoError(t, dag.AddEvent(ctx, sub1a))
		require.NoError(t, dag.AddEvent(ctx, sub1b))

		// Create sub-events for main2
		sub2a := &Event{
			ID:       "sub2a",
			Type:     SubEvent,
			ParentID: "main2",
		}
		sub2b := &Event{
			ID:       "sub2b",
			Type:     SubEvent,
			ParentID: "main2",
			Parents:  []string{"sub1b", "sub2a"}, // Cross-event relationship
		}
		require.NoError(t, dag.AddEvent(ctx, sub2a))
		require.NoError(t, dag.AddEvent(ctx, sub2b))

		// Test GetSubEvents
		sub1Events, err := dag.GetSubEvents(ctx, "main1")
		assert.NoError(t, err)
		assert.Len(t, sub1Events, 2)
		assert.Contains(t, []string{sub1Events[0].ID, sub1Events[1].ID}, "sub1a")
		assert.Contains(t, []string{sub1Events[0].ID, sub1Events[1].ID}, "sub1b")

		// Test GetChildren
		children, err := dag.GetChildren(ctx, "main1")
		assert.NoError(t, err)
		assert.Len(t, children, 2)

		// Test GetMainEvents
		mainEvents, err := dag.GetMainEvents(ctx)
		assert.NoError(t, err)
		assert.Len(t, mainEvents, 2)
	})

	t.Run("Verify", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Create valid event structure
		events := []*Event{
			{ID: "main1", Type: MainEvent},
			{ID: "main2", Type: MainEvent, Parents: []string{"main1"}},
			{ID: "sub1", Type: SubEvent, ParentID: "main1"},
			{ID: "sub2", Type: SubEvent, ParentID: "main2", Parents: []string{"sub1"}},
		}

		for _, event := range events {
			require.NoError(t, dag.AddEvent(ctx, event))
		}

		// Test valid DAG
		assert.NoError(t, dag.Verify(ctx))

		// Test cycle detection
		dag.(*memoryDAG).events["sub1"].Parents = append(
			dag.(*memoryDAG).events["sub1"].Parents,
			"sub2",
		)
		assert.Error(t, dag.Verify(ctx))
	})
}
