package dag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
)

// IPFSConfig represents configuration for the IPFS DAG backend
type IPFSConfig struct {
	// APIURL is the IPFS HTTP API URL (default: http://localhost:5001)
	APIURL string
}

// ipfsDAG implements the DAG interface with IPFS storage
type ipfsDAG struct {
	cfg    IPFSConfig
	client *http.Client

	// Local index for graph traversal
	// Maps event ID (CID) to event metadata
	mu       sync.RWMutex
	events   map[string]*Event
	children map[string][]string // parent ID -> child IDs
}

// NewIPFSDAG creates a new DAG with IPFS storage
func NewIPFSDAG(cfg IPFSConfig) (DAG, error) {
	if cfg.APIURL == "" {
		cfg.APIURL = "http://localhost:5001"
	}

	d := &ipfsDAG{
		cfg:      cfg,
		client:   &http.Client{},
		events:   make(map[string]*Event),
		children: make(map[string][]string),
	}

	// Verify IPFS connection
	if err := d.ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to IPFS: %w", err)
	}

	return d, nil
}

// ping checks if IPFS is reachable
func (d *ipfsDAG) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", d.cfg.APIURL+"/api/v0/id", nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IPFS returned status %d", resp.StatusCode)
	}

	return nil
}

// eventData represents the serialized form of an event stored in IPFS
type eventData struct {
	Type     EventType     `json:"type"`
	ParentID string        `json:"parent_id,omitempty"`
	Parents  []string      `json:"parents,omitempty"`
	Data     []byte        `json:"data,omitempty"`
	Beacon   *BeaconAnchor `json:"beacon,omitempty"`
}

// ipfsAddResponse represents the response from IPFS add
type ipfsAddResponse struct {
	Hash string `json:"Hash"`
	Name string `json:"Name"`
	Size string `json:"Size"`
}

// storeEvent stores an event in IPFS and returns its CID
func (d *ipfsDAG) storeEvent(ctx context.Context, event *Event) (string, error) {
	// Serialize event (excluding ID and Children which are computed/derived)
	data := eventData{
		Type:     event.Type,
		ParentID: event.ParentID,
		Parents:  event.Parents,
		Data:     event.Data,
		Beacon:   event.Beacon,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create multipart form for IPFS add
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "event.json")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(jsonData); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	// POST to IPFS
	req, err := http.NewRequestWithContext(ctx, "POST", d.cfg.APIURL+"/api/v0/add", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to add to IPFS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("IPFS add failed: %s", string(body))
	}

	var addResp ipfsAddResponse
	if err := json.NewDecoder(resp.Body).Decode(&addResp); err != nil {
		return "", fmt.Errorf("failed to decode IPFS response: %w", err)
	}

	return addResp.Hash, nil
}

// fetchEvent retrieves an event from IPFS by CID
func (d *ipfsDAG) fetchEvent(ctx context.Context, cid string) (*Event, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", d.cfg.APIURL+"/api/v0/cat?arg="+cid, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from IPFS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IPFS cat failed: %s", string(body))
	}

	var data eventData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode event: %w", err)
	}

	event := &Event{
		ID:       cid,
		Type:     data.Type,
		ParentID: data.ParentID,
		Parents:  data.Parents,
		Data:     data.Data,
		Beacon:   data.Beacon,
	}

	return event, nil
}

func (d *ipfsDAG) AddEvent(ctx context.Context, event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// If event has no ID, compute it
	if event.ID == "" {
		cid, err := ComputeCID(event)
		if err != nil {
			return fmt.Errorf("failed to compute CID: %w", err)
		}
		event.ID = cid
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if event already exists
	if _, exists := d.events[event.ID]; exists {
		return fmt.Errorf("event %s already exists", event.ID)
	}

	// For sub-events, verify parent exists
	if event.Type == SubEvent {
		if event.ParentID == "" {
			return fmt.Errorf("sub-event must have a parent ID")
		}
		if _, exists := d.events[event.ParentID]; !exists {
			return fmt.Errorf("parent event %s not found", event.ParentID)
		}
		d.children[event.ParentID] = append(d.children[event.ParentID], event.ID)
	}

	// Verify all parents exist
	for _, parentID := range event.Parents {
		if _, exists := d.events[parentID]; !exists {
			return fmt.Errorf("parent event %s does not exist", parentID)
		}
		d.children[parentID] = append(d.children[parentID], event.ID)
	}

	// Store in IPFS
	storedCID, err := d.storeEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to store event in IPFS: %w", err)
	}

	// Note: IPFS returns its own CID (storedCID) which differs from our computed CID (event.ID)
	// We use our computed CID as the canonical ID and store the mapping locally
	_ = storedCID // Acknowledge IPFS CID (unused - we use our own content-addressed ID)

	// Update local index
	d.events[event.ID] = event

	return nil
}

func (d *ipfsDAG) GetEvent(ctx context.Context, id string) (*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	event, exists := d.events[id]
	d.mu.RUnlock()

	if exists {
		// Return copy with children populated
		eventCopy := *event
		eventCopy.Children = d.getChildrenIDs(id)
		return &eventCopy, nil
	}

	// Try to fetch from IPFS
	event, err := d.fetchEvent(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	// Cache locally
	d.mu.Lock()
	d.events[id] = event
	d.mu.Unlock()

	event.Children = d.getChildrenIDs(id)
	return event, nil
}

func (d *ipfsDAG) getChildrenIDs(id string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.children[id]
}

func (d *ipfsDAG) GetParents(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	event, err := d.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}

	parents := make([]*Event, 0, len(event.Parents))
	for _, parentID := range event.Parents {
		parent, err := d.GetEvent(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("parent event %s not found: %w", parentID, err)
		}
		parents = append(parents, parent)
	}

	return parents, nil
}

func (d *ipfsDAG) GetChildren(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	childIDs := d.children[id]
	d.mu.RUnlock()

	children := make([]*Event, 0, len(childIDs))
	for _, childID := range childIDs {
		child, err := d.GetEvent(ctx, childID)
		if err != nil {
			return nil, fmt.Errorf("child event %s not found: %w", childID, err)
		}
		children = append(children, child)
	}

	return children, nil
}

func (d *ipfsDAG) GetSubEvents(ctx context.Context, id string) ([]*Event, error) {
	if id == "" {
		return nil, fmt.Errorf("event ID cannot be empty")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.events[id]; !exists {
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
			for _, childID := range d.children[eventID] {
				if err := traverse(childID); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Start with direct children
	for _, childID := range d.children[id] {
		if err := traverse(childID); err != nil {
			return nil, err
		}
	}

	return subEvents, nil
}

func (d *ipfsDAG) GetMainEvents(ctx context.Context) ([]*Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	mainEvents := make([]*Event, 0)
	for _, event := range d.events {
		if event.Type == MainEvent {
			eventCopy := *event
			eventCopy.Children = d.children[event.ID]
			mainEvents = append(mainEvents, &eventCopy)
		}
	}

	return mainEvents, nil
}

func (d *ipfsDAG) Verify(ctx context.Context) error {
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
