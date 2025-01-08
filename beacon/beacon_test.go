package beacon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBeacon(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				URL:       "http://example.com",
				ChainHash: []byte("chain-hash"),
				PublicKey: []byte("public-key"),
				Period:    time.Second,
			},
			wantErr: false,
		},
		{
			name: "empty URL",
			cfg: Config{
				ChainHash: []byte("chain-hash"),
				PublicKey: []byte("public-key"),
				Period:    time.Second,
			},
			wantErr: true,
		},
		{
			name: "empty chain hash",
			cfg: Config{
				URL:       "http://example.com",
				PublicKey: []byte("public-key"),
				Period:    time.Second,
			},
			wantErr: true,
		},
		{
			name: "empty public key",
			cfg: Config{
				URL:       "http://example.com",
				ChainHash: []byte("chain-hash"),
				Period:    time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero period",
			cfg: Config{
				URL:       "http://example.com",
				ChainHash: []byte("chain-hash"),
				PublicKey: []byte("public-key"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBeacon(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDrandBeacon(t *testing.T) {
	// Mock drand server
	mockRound := drandResponse{
		Round:      123,
		Randomness: hex.EncodeToString([]byte("random-bytes")),
		Signature:  hex.EncodeToString([]byte("signature-bytes")),
		Timestamp:  float64(time.Now().Unix()),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(mockRound); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	// Create beacon
	cfg := Config{
		URL:       server.URL,
		ChainHash: []byte("chain-hash"),
		PublicKey: []byte("public-key"),
		Period:    time.Second,
	}

	b, err := NewBeacon(cfg)
	require.NoError(t, err)

	// Test GetLatestRound
	t.Run("GetLatestRound", func(t *testing.T) {
		round, err := b.GetLatestRound(context.Background())
		require.NoError(t, err)
		assert.Equal(t, mockRound.Round, round.Number)
		assert.Equal(t, []byte("random-bytes"), round.Randomness)
		assert.Equal(t, []byte("signature-bytes"), round.Signature)
	})

	// Test GetRound
	t.Run("GetRound", func(t *testing.T) {
		round, err := b.GetRound(context.Background(), 123)
		require.NoError(t, err)
		assert.Equal(t, mockRound.Round, round.Number)
		assert.Equal(t, []byte("random-bytes"), round.Randomness)
		assert.Equal(t, []byte("signature-bytes"), round.Signature)
	})

	// Test Subscribe and Start
	t.Run("Subscribe", func(t *testing.T) {
		ch, err := b.Subscribe()
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = b.Start(ctx, 100*time.Millisecond)
		require.NoError(t, err)

		// Should receive at least one round
		select {
		case round := <-ch:
			assert.Equal(t, mockRound.Round, round.Number)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for round")
		}

		// Stop should work
		err = b.Stop()
		require.NoError(t, err)
	})
}
