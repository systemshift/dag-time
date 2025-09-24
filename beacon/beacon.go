// Package beacon provides integration with drand for global time anchoring
package beacon

import (
	"context"
	"fmt"
	"time"
)

// Round represents a drand beacon round
type Round struct {
	// Number is the round number
	Number uint64

	// Randomness is the random bytes produced by drand
	Randomness []byte

	// Signature is the threshold signature of the round
	Signature []byte

	// Timestamp is when this round was created
	Timestamp time.Time
}

// Beacon represents a drand beacon client
type Beacon interface {
	// GetLatestRound fetches the latest beacon round
	GetLatestRound(ctx context.Context) (*Round, error)

	// GetRound fetches a specific beacon round
	GetRound(ctx context.Context, round uint64) (*Round, error)

	// Start begins fetching rounds at the specified interval
	Start(ctx context.Context, interval time.Duration) error

	// Stop stops fetching rounds
	Stop() error

	// Subscribe returns a channel that receives new rounds
	Subscribe() (<-chan *Round, error)
}

// Config represents beacon configuration
type Config struct {
	// URL is the drand HTTP endpoint
	URL string

	// ChainHash is the hash of the drand chain info
	ChainHash []byte

	// PublicKey is the drand group public key
	PublicKey []byte

	// Period is the duration between rounds
	Period time.Duration

	// GenesisTime is the UNIX timestamp when the drand network started
	GenesisTime int64
}

// ErrInvalidConfig indicates the beacon configuration is invalid
var ErrInvalidConfig = fmt.Errorf("invalid beacon configuration")

// NewBeacon creates a new drand beacon client
func NewBeacon(cfg Config) (Beacon, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("%w: URL cannot be empty", ErrInvalidConfig)
	}
	if len(cfg.ChainHash) == 0 {
		return nil, fmt.Errorf("%w: ChainHash cannot be empty", ErrInvalidConfig)
	}
	if len(cfg.PublicKey) == 0 {
		return nil, fmt.Errorf("%w: PublicKey cannot be empty", ErrInvalidConfig)
	}
	if cfg.Period <= 0 {
		return nil, fmt.Errorf("%w: Period must be positive", ErrInvalidConfig)
	}
	if cfg.GenesisTime <= 0 {
		return nil, fmt.Errorf("%w: GenesisTime must be positive", ErrInvalidConfig)
	}

	return newDrandBeacon(cfg)
}
