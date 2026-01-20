package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeCID(t *testing.T) {
	t.Run("same content produces same CID", func(t *testing.T) {
		event1 := &Event{
			Type:     MainEvent,
			ParentID: "",
			Parents:  []string{"parent1", "parent2"},
			Data:     []byte("test data"),
		}
		event2 := &Event{
			Type:     MainEvent,
			ParentID: "",
			Parents:  []string{"parent1", "parent2"},
			Data:     []byte("test data"),
		}

		cid1, err := ComputeCID(event1)
		require.NoError(t, err)

		cid2, err := ComputeCID(event2)
		require.NoError(t, err)

		assert.Equal(t, cid1, cid2)
	})

	t.Run("different data produces different CID", func(t *testing.T) {
		event1 := &Event{
			Type: MainEvent,
			Data: []byte("data 1"),
		}
		event2 := &Event{
			Type: MainEvent,
			Data: []byte("data 2"),
		}

		cid1, err := ComputeCID(event1)
		require.NoError(t, err)

		cid2, err := ComputeCID(event2)
		require.NoError(t, err)

		assert.NotEqual(t, cid1, cid2)
	})

	t.Run("parent order does not affect CID", func(t *testing.T) {
		event1 := &Event{
			Type:    MainEvent,
			Parents: []string{"a", "b", "c"},
			Data:    []byte("test"),
		}
		event2 := &Event{
			Type:    MainEvent,
			Parents: []string{"c", "a", "b"},
			Data:    []byte("test"),
		}

		cid1, err := ComputeCID(event1)
		require.NoError(t, err)

		cid2, err := ComputeCID(event2)
		require.NoError(t, err)

		assert.Equal(t, cid1, cid2, "CID should be deterministic regardless of parent order")
	})

	t.Run("children do not affect CID", func(t *testing.T) {
		event1 := &Event{
			Type:     MainEvent,
			Data:     []byte("test"),
			Children: []string{},
		}
		event2 := &Event{
			Type:     MainEvent,
			Data:     []byte("test"),
			Children: []string{"child1", "child2"},
		}

		cid1, err := ComputeCID(event1)
		require.NoError(t, err)

		cid2, err := ComputeCID(event2)
		require.NoError(t, err)

		assert.Equal(t, cid1, cid2, "Children should not affect CID")
	})

	t.Run("beacon does not affect CID", func(t *testing.T) {
		event1 := &Event{
			Type: MainEvent,
			Data: []byte("test"),
		}
		event2 := &Event{
			Type: MainEvent,
			Data: []byte("test"),
			Beacon: &BeaconAnchor{
				Round:      100,
				Randomness: []byte("random"),
			},
		}

		cid1, err := ComputeCID(event1)
		require.NoError(t, err)

		cid2, err := ComputeCID(event2)
		require.NoError(t, err)

		assert.Equal(t, cid1, cid2, "Beacon should not affect CID")
	})

	t.Run("nil event returns error", func(t *testing.T) {
		_, err := ComputeCID(nil)
		assert.Error(t, err)
	})

	t.Run("CID format is valid", func(t *testing.T) {
		event := &Event{
			Type: MainEvent,
			Data: []byte("test"),
		}

		cid, err := ComputeCID(event)
		require.NoError(t, err)

		// CIDv1 with raw codec and SHA-256 starts with 'b' (base32)
		assert.True(t, len(cid) > 0)
		// Verify it's a valid base32 CID (starts with 'b')
		assert.Equal(t, "b", cid[:1], "CIDv1 should use base32 encoding")
	})
}

func TestVerifyCID(t *testing.T) {
	t.Run("valid CID passes verification", func(t *testing.T) {
		event := &Event{
			Type: MainEvent,
			Data: []byte("test"),
		}

		cid, err := ComputeCID(event)
		require.NoError(t, err)

		event.ID = cid

		err = VerifyCID(event)
		assert.NoError(t, err)
	})

	t.Run("invalid CID fails verification", func(t *testing.T) {
		event := &Event{
			ID:   "invalid-cid",
			Type: MainEvent,
			Data: []byte("test"),
		}

		err := VerifyCID(event)
		assert.Error(t, err)
	})

	t.Run("modified event fails verification", func(t *testing.T) {
		event := &Event{
			Type: MainEvent,
			Data: []byte("original"),
		}

		cid, err := ComputeCID(event)
		require.NoError(t, err)

		event.ID = cid
		event.Data = []byte("modified") // Tamper with data

		err = VerifyCID(event)
		assert.Error(t, err)
	})

	t.Run("nil event returns error", func(t *testing.T) {
		err := VerifyCID(nil)
		assert.Error(t, err)
	})
}
