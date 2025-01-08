package dag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

		// Test adding valid event
		event := &Event{ID: "event1", Data: []byte("test")}
		err = dag.AddEvent(ctx, event)
		assert.NoError(t, err)

		// Test adding duplicate event
		err = dag.AddEvent(ctx, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// Test adding event with non-existent parent
		event2 := &Event{ID: "event2", Parents: []string{"nonexistent"}}
		err = dag.AddEvent(ctx, event2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parent event nonexistent does not exist")
	})

	t.Run("GetEvent", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Test getting non-existent event
		_, err := dag.GetEvent(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")

		// Test getting empty ID
		_, err = dag.GetEvent(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")

		// Test getting existing event
		event := &Event{ID: "event1", Data: []byte("test")}
		err = dag.AddEvent(ctx, event)
		assert.NoError(t, err)

		retrieved, err := dag.GetEvent(ctx, "event1")
		assert.NoError(t, err)
		assert.Equal(t, event, retrieved)
	})

	t.Run("GetParents", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Add parent event
		parent := &Event{ID: "parent", Data: []byte("parent")}
		err := dag.AddEvent(ctx, parent)
		assert.NoError(t, err)

		// Add child event
		child := &Event{ID: "child", Parents: []string{"parent"}, Data: []byte("child")}
		err = dag.AddEvent(ctx, child)
		assert.NoError(t, err)

		// Test getting parents
		parents, err := dag.GetParents(ctx, "child")
		assert.NoError(t, err)
		assert.Len(t, parents, 1)
		assert.Equal(t, parent, parents[0])

		// Test getting parents of non-existent event
		_, err = dag.GetParents(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Verify", func(t *testing.T) {
		dag := NewMemoryDAG()

		// Test empty DAG
		err := dag.Verify(ctx)
		assert.NoError(t, err)

		// Add events forming a valid DAG
		events := []*Event{
			{ID: "1", Data: []byte("1")},
			{ID: "2", Parents: []string{"1"}, Data: []byte("2")},
			{ID: "3", Parents: []string{"1"}, Data: []byte("3")},
			{ID: "4", Parents: []string{"2", "3"}, Data: []byte("4")},
		}

		for _, event := range events {
			err := dag.AddEvent(ctx, event)
			assert.NoError(t, err)
		}

		// Verify valid DAG
		err = dag.Verify(ctx)
		assert.NoError(t, err)

		// Test cycle detection by manually creating a cycle
		dag.(*memoryDAG).events["2"].Parents = append(dag.(*memoryDAG).events["2"].Parents, "4")
		err = dag.Verify(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})
}
