package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// DAGSnapshot represents a serializable snapshot of the DAG state
type DAGSnapshot struct {
	Version  int                 `json:"version"`
	Events   map[string]*Event   `json:"events"`
	Children map[string][]string `json:"children,omitempty"`
}

// CurrentSnapshotVersion is the current snapshot format version
const CurrentSnapshotVersion = 1

// Exporter provides export/import functionality for a DAG
type Exporter interface {
	// Export writes the DAG state to the provided writer
	Export(ctx context.Context, w io.Writer) error
	// Import reads the DAG state from the provided reader
	Import(ctx context.Context, r io.Reader) error
}

// ExportDAG exports the DAG state to the provided writer using the given events and children maps
func ExportDAG(ctx context.Context, w io.Writer, events map[string]*Event, children map[string][]string) error {
	snapshot := DAGSnapshot{
		Version:  CurrentSnapshotVersion,
		Events:   events,
		Children: children,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}

	return nil
}

// ImportDAG reads the DAG state from the provided reader and returns the events and children maps
func ImportDAG(ctx context.Context, r io.Reader) (map[string]*Event, map[string][]string, error) {
	var snapshot DAGSnapshot
	if err := json.NewDecoder(r).Decode(&snapshot); err != nil {
		return nil, nil, fmt.Errorf("failed to decode snapshot: %w", err)
	}

	if snapshot.Version != CurrentSnapshotVersion {
		return nil, nil, fmt.Errorf("unsupported snapshot version: %d (expected %d)", snapshot.Version, CurrentSnapshotVersion)
	}

	if snapshot.Events == nil {
		snapshot.Events = make(map[string]*Event)
	}
	if snapshot.Children == nil {
		snapshot.Children = make(map[string][]string)
	}

	return snapshot.Events, snapshot.Children, nil
}
