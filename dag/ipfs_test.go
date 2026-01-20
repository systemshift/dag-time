package dag

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ipfsAvailable() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://localhost:5001/api/v0/id", "", nil)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func TestIPFSDAG(t *testing.T) {
	if !ipfsAvailable() {
		t.Skip("IPFS not available, skipping IPFS tests")
	}

	t.Run("NewIPFSDAG", func(t *testing.T) {
		dag, err := NewIPFSDAG(IPFSConfig{})
		require.NoError(t, err)
		assert.NotNil(t, dag)
	})

	t.Run("AddEvent", func(t *testing.T) {
		dag, err := NewIPFSDAG(IPFSConfig{})
		require.NoError(t, err)

		event := &Event{
			Type: MainEvent,
			Data: []byte("test data"),
		}

		// Compute CID
		cid, err := ComputeCID(event)
		require.NoError(t, err)
		event.ID = cid

		ctx := context.Background()
		err = dag.AddEvent(ctx, event)
		require.NoError(t, err)

		// Retrieve event
		retrieved, err := dag.GetEvent(ctx, event.ID)
		require.NoError(t, err)
		assert.Equal(t, event.ID, retrieved.ID)
		assert.Equal(t, event.Type, retrieved.Type)
		assert.Equal(t, event.Data, retrieved.Data)
	})

	t.Run("AddSubEvent", func(t *testing.T) {
		dag, err := NewIPFSDAG(IPFSConfig{})
		require.NoError(t, err)

		ctx := context.Background()

		// Create main event
		mainEvent := &Event{
			Type: MainEvent,
			Data: []byte("main event"),
		}
		cid, err := ComputeCID(mainEvent)
		require.NoError(t, err)
		mainEvent.ID = cid
		err = dag.AddEvent(ctx, mainEvent)
		require.NoError(t, err)

		// Create sub-event
		subEvent := &Event{
			Type:     SubEvent,
			ParentID: mainEvent.ID,
			Data:     []byte("sub event"),
		}
		cid, err = ComputeCID(subEvent)
		require.NoError(t, err)
		subEvent.ID = cid
		err = dag.AddEvent(ctx, subEvent)
		require.NoError(t, err)

		// Verify relationship
		subEvents, err := dag.GetSubEvents(ctx, mainEvent.ID)
		require.NoError(t, err)
		assert.Len(t, subEvents, 1)
		assert.Equal(t, subEvent.ID, subEvents[0].ID)
	})

	t.Run("GetParents", func(t *testing.T) {
		dag, err := NewIPFSDAG(IPFSConfig{})
		require.NoError(t, err)

		ctx := context.Background()

		// Create parent event
		parent := &Event{
			Type: MainEvent,
			Data: []byte("parent"),
		}
		cid, err := ComputeCID(parent)
		require.NoError(t, err)
		parent.ID = cid
		err = dag.AddEvent(ctx, parent)
		require.NoError(t, err)

		// Create child event with parent reference
		child := &Event{
			Type:    MainEvent,
			Parents: []string{parent.ID},
			Data:    []byte("child"),
		}
		cid, err = ComputeCID(child)
		require.NoError(t, err)
		child.ID = cid
		err = dag.AddEvent(ctx, child)
		require.NoError(t, err)

		// Get parents
		parents, err := dag.GetParents(ctx, child.ID)
		require.NoError(t, err)
		assert.Len(t, parents, 1)
		assert.Equal(t, parent.ID, parents[0].ID)
	})

	t.Run("Verify", func(t *testing.T) {
		dag, err := NewIPFSDAG(IPFSConfig{})
		require.NoError(t, err)

		ctx := context.Background()

		// Create a simple DAG
		event1 := &Event{
			Type: MainEvent,
			Data: []byte("event 1"),
		}
		cid, err := ComputeCID(event1)
		require.NoError(t, err)
		event1.ID = cid
		err = dag.AddEvent(ctx, event1)
		require.NoError(t, err)

		event2 := &Event{
			Type:    MainEvent,
			Parents: []string{event1.ID},
			Data:    []byte("event 2"),
		}
		cid, err = ComputeCID(event2)
		require.NoError(t, err)
		event2.ID = cid
		err = dag.AddEvent(ctx, event2)
		require.NoError(t, err)

		// Verify should pass
		err = dag.Verify(ctx)
		assert.NoError(t, err)
	})
}

func TestIPFSDAGConnectionFailure(t *testing.T) {
	// Test with invalid IPFS URL
	_, err := NewIPFSDAG(IPFSConfig{
		APIURL: "http://localhost:59999", // Non-existent port
	})
	assert.Error(t, err)
}
