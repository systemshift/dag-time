package dag

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// ComputeCID computes a content-addressed identifier for an event.
// The CID is computed from the immutable parts of the event:
// - Type
// - ParentID
// - Parents (sorted for determinism)
// - Data
//
// Children and Beacon are NOT included as they are added after creation.
func ComputeCID(event *Event) (string, error) {
	if event == nil {
		return "", fmt.Errorf("event cannot be nil")
	}

	// Serialize event content deterministically
	data, err := serializeEventContent(event)
	if err != nil {
		return "", fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create multihash using SHA-256
	mh, err := multihash.Sum(data, multihash.SHA2_256, -1)
	if err != nil {
		return "", fmt.Errorf("failed to create multihash: %w", err)
	}

	// Create CID v1 with raw codec
	c := cid.NewCidV1(cid.Raw, mh)

	return c.String(), nil
}

// serializeEventContent creates a deterministic byte representation of event content.
// Uses a simple binary format:
// - 1 byte: event type
// - 4 bytes: parent ID length, then parent ID bytes
// - 4 bytes: number of parents, then sorted parent IDs
// - 4 bytes: data length, then data bytes
func serializeEventContent(event *Event) ([]byte, error) {
	var buf bytes.Buffer

	// Write event type (1 byte)
	if err := buf.WriteByte(byte(event.Type)); err != nil {
		return nil, err
	}

	// Write parent ID
	if err := writeString(&buf, event.ParentID); err != nil {
		return nil, err
	}

	// Write parents (sorted for determinism)
	sortedParents := make([]string, len(event.Parents))
	copy(sortedParents, event.Parents)
	sort.Strings(sortedParents)

	if err := binary.Write(&buf, binary.BigEndian, uint32(len(sortedParents))); err != nil {
		return nil, err
	}
	for _, parent := range sortedParents {
		if err := writeString(&buf, parent); err != nil {
			return nil, err
		}
	}

	// Write data
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(event.Data))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(event.Data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// writeString writes a length-prefixed string to the buffer
func writeString(buf *bytes.Buffer, s string) error {
	if err := binary.Write(buf, binary.BigEndian, uint32(len(s))); err != nil {
		return err
	}
	if _, err := buf.WriteString(s); err != nil {
		return err
	}
	return nil
}

// VerifyCID checks if an event's ID matches its computed CID.
func VerifyCID(event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	computed, err := ComputeCID(event)
	if err != nil {
		return fmt.Errorf("failed to compute CID: %w", err)
	}

	if event.ID != computed {
		return fmt.Errorf("CID mismatch: expected %s, got %s", computed, event.ID)
	}

	return nil
}
