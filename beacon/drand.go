package beacon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// drandBeacon implements the Beacon interface using drand's HTTP API
type drandBeacon struct {
	cfg    Config
	client *http.Client

	mu          sync.RWMutex
	subscribers []chan *Round
	running     bool
	cancel      context.CancelFunc
}

type drandResponse struct {
	Round      uint64  `json:"round"`
	Randomness string  `json:"randomness"`
	Signature  string  `json:"signature"`
	Timestamp  float64 `json:"previous_signature"` // Unix timestamp
}

func newDrandBeacon(cfg Config) (*drandBeacon, error) {
	return &drandBeacon{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		subscribers: make([]chan *Round, 0),
	}, nil
}

func (d *drandBeacon) GetLatestRound(ctx context.Context) (*Round, error) {
	resp, err := d.client.Get(fmt.Sprintf("%s/public/latest", d.cfg.URL))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest round: %w", err)
	}
	defer resp.Body.Close()

	return d.parseResponse(resp.Body)
}

func (d *drandBeacon) GetRound(ctx context.Context, round uint64) (*Round, error) {
	resp, err := d.client.Get(fmt.Sprintf("%s/public/%d", d.cfg.URL, round))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch round %d: %w", round, err)
	}
	defer resp.Body.Close()

	return d.parseResponse(resp.Body)
}

func (d *drandBeacon) parseResponse(r io.Reader) (*Round, error) {
	var resp drandResponse
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	randomness, err := hex.DecodeString(resp.Randomness)
	if err != nil {
		return nil, fmt.Errorf("failed to decode randomness: %w", err)
	}

	signature, err := hex.DecodeString(resp.Signature)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	return &Round{
		Number:     resp.Round,
		Randomness: randomness,
		Signature:  signature,
		Timestamp:  time.Unix(int64(resp.Timestamp), 0),
	}, nil
}

func (d *drandBeacon) Start(ctx context.Context, interval time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return fmt.Errorf("beacon already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.running = true

	go d.run(ctx, interval)
	return nil
}

func (d *drandBeacon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.cancel()
	d.running = false
	return nil
}

func (d *drandBeacon) Subscribe() (<-chan *Round, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan *Round, 1)
	d.subscribers = append(d.subscribers, ch)
	return ch, nil
}

func (d *drandBeacon) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			round, err := d.GetLatestRound(ctx)
			if err != nil {
				continue
			}

			d.mu.RLock()
			for _, ch := range d.subscribers {
				select {
				case ch <- round:
				default:
					// Skip if subscriber is not ready
				}
			}
			d.mu.RUnlock()
		}
	}
}
