package beacon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Beacon represents a global time source
type Beacon interface {
	// GetLatestRound fetches the latest round information from the beacon
	GetLatestRound(ctx context.Context) (*Round, error)

	// Start begins periodic beacon fetching
	Start(ctx context.Context, interval time.Duration) error

	// Stop halts beacon fetching
	Stop() error
}

// Round represents a single beacon round
type Round struct {
	Number     uint64
	Randomness []byte
	Signature  []byte
	Timestamp  time.Time
}

// DrandBeacon implements the Beacon interface using drand
type DrandBeacon struct {
	endpoint  string
	client    *http.Client
	roundChan chan *Round
	stopChan  chan struct{}
	isRunning bool
}

// NewDrandBeacon creates a new drand beacon client
func NewDrandBeacon(endpoint string) (*DrandBeacon, error) {
	if endpoint == "" {
		endpoint = "https://api.drand.sh"
	}

	return &DrandBeacon{
		endpoint:  endpoint,
		client:    &http.Client{Timeout: 10 * time.Second},
		roundChan: make(chan *Round),
		stopChan:  make(chan struct{}),
	}, nil
}

// GetLatestRound implements Beacon.GetLatestRound
func (d *DrandBeacon) GetLatestRound(ctx context.Context) (*Round, error) {
	url := fmt.Sprintf("%s/public/latest", d.endpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest round: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Round      uint64 `json:"round"`
		Randomness string `json:"randomness"`
		Signature  string `json:"signature"`
		Timestamp  uint64 `json:"timestamp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	randomness, err := hex.DecodeString(result.Randomness)
	if err != nil {
		return nil, fmt.Errorf("decoding randomness: %w", err)
	}

	signature, err := hex.DecodeString(result.Signature)
	if err != nil {
		return nil, fmt.Errorf("decoding signature: %w", err)
	}

	return &Round{
		Number:     result.Round,
		Randomness: randomness,
		Signature:  signature,
		Timestamp:  time.Unix(int64(result.Timestamp), 0),
	}, nil
}

// Start implements Beacon.Start
func (d *DrandBeacon) Start(ctx context.Context, interval time.Duration) error {
	if d.isRunning {
		return nil
	}

	d.isRunning = true

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				d.isRunning = false
				return
			case <-d.stopChan:
				d.isRunning = false
				return
			case <-ticker.C:
				round, err := d.GetLatestRound(ctx)
				if err != nil {
					// TODO: Add proper logging
					continue
				}

				select {
				case d.roundChan <- round:
				default:
					// Channel full, skip this round
				}
			}
		}
	}()

	return nil
}

// Stop implements Beacon.Stop
func (d *DrandBeacon) Stop() error {
	if !d.isRunning {
		return nil
	}

	close(d.stopChan)
	return nil
}

// Rounds returns a channel that receives new rounds
func (d *DrandBeacon) Rounds() <-chan *Round {
	return d.roundChan
}
