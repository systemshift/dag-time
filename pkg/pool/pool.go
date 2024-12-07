package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	TopicName = "dag-time-pool"
)

// PoolEvent represents an event in the temporary pool
type PoolEvent struct {
	ID        string    `json:"id"`
	Data      []byte    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
	Creator   string    `json:"creator"` // Peer ID of the creator
	Parents   []string  `json:"parents"` // IDs of parent events
	Proof     []byte    `json:"proof"`   // Proof of work
}

// Pool manages the temporary event pool
type Pool struct {
	mu     sync.RWMutex
	host   host.Host
	pubsub *pubsub.PubSub
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
	events map[string]*PoolEvent // Map of event ID to event
	peers  map[peer.ID]time.Time // Map of peer ID to last seen time
}

// NewPool creates a new temporary event pool
func NewPool(ctx context.Context, h host.Host) (*Pool, error) {
	log.Printf("Creating new pool with host ID: %s", h.ID())

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub: %w", err)
	}
	log.Printf("Created GossipSub instance")

	// Join topic
	topic, err := ps.Join(TopicName)
	if err != nil {
		return nil, fmt.Errorf("joining topic: %w", err)
	}
	log.Printf("Joined topic: %s", TopicName)

	// Subscribe to topic
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribing to topic: %w", err)
	}
	log.Printf("Subscribed to topic")

	pool := &Pool{
		host:   h,
		pubsub: ps,
		topic:  topic,
		sub:    sub,
		events: make(map[string]*PoolEvent),
		peers:  make(map[peer.ID]time.Time),
	}

	// Start handling messages
	go pool.handleMessages(ctx)
	log.Printf("Started message handler")

	return pool, nil
}

// handleMessages processes incoming messages from the pubsub
func (p *Pool) handleMessages(ctx context.Context) {
	log.Printf("Message handler started for host %s", p.host.ID())
	for {
		msg, err := p.sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("Context cancelled, stopping message handler")
				return // Context cancelled
			}
			log.Printf("Error getting next message: %v", err)
			continue
		}

		// Skip messages from ourselves
		if msg.ReceivedFrom == p.host.ID() {
			log.Printf("Skipping message from self")
			continue
		}

		// Update peer last seen time
		p.mu.Lock()
		p.peers[msg.ReceivedFrom] = time.Now()
		p.mu.Unlock()

		// Decode event
		var event PoolEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("Error decoding message: %v", err)
			continue
		}

		// Add event to pool
		p.mu.Lock()
		p.events[event.ID] = &event
		p.mu.Unlock()

		log.Printf("Received event %s from peer %s", event.ID, msg.ReceivedFrom)
	}
}

// AddEvent adds a new event to the pool and broadcasts it
func (p *Pool) AddEvent(ctx context.Context, event *PoolEvent) error {
	// Verify event has required fields
	if event.ID == "" || event.Creator == "" {
		return fmt.Errorf("event missing required fields")
	}

	log.Printf("Adding event %s from creator %s", event.ID, event.Creator)

	// Add event to local pool
	p.mu.Lock()
	p.events[event.ID] = event
	p.mu.Unlock()

	// Broadcast event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}

	if err := p.topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("publishing event: %w", err)
	}

	log.Printf("Published event %s to topic", event.ID)
	return nil
}

// GetEvents returns all events in the pool
func (p *Pool) GetEvents() []*PoolEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	events := make([]*PoolEvent, 0, len(p.events))
	for _, event := range p.events {
		events = append(events, event)
	}
	return events
}

// GetPeers returns all known peers and their last seen time
func (p *Pool) GetPeers() map[peer.ID]time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers := make(map[peer.ID]time.Time, len(p.peers))
	for id, lastSeen := range p.peers {
		peers[id] = lastSeen
	}
	return peers
}

// Close shuts down the pool
func (p *Pool) Close() error {
	log.Printf("Closing pool")
	p.sub.Cancel()
	return p.topic.Close()
}
