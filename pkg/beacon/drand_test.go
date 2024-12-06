package beacon

import (
	"context"
	"testing"
	"time"
)

func TestNewDrandBeacon(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{
			name:     "default endpoint",
			endpoint: "",
			wantErr:  false,
		},
		{
			name:     "custom endpoint",
			endpoint: "https://api.drand.sh",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beacon, err := NewDrandBeacon(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDrandBeacon() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if beacon == nil {
				t.Error("NewDrandBeacon() returned nil beacon")
			}
		})
	}
}

func TestDrandBeacon_GetLatestRound(t *testing.T) {
	beacon, err := NewDrandBeacon("")
	if err != nil {
		t.Fatalf("Failed to create beacon: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	round, err := beacon.GetLatestRound(ctx)
	if err != nil {
		t.Fatalf("GetLatestRound() error = %v", err)
	}

	if round == nil {
		t.Fatal("GetLatestRound() returned nil round")
	}

	if round.Number == 0 {
		t.Error("GetLatestRound() returned round with number 0")
	}

	if len(round.Randomness) == 0 {
		t.Error("GetLatestRound() returned round with empty randomness")
	}

	if len(round.Signature) == 0 {
		t.Error("GetLatestRound() returned round with empty signature")
	}

	if round.Timestamp.IsZero() {
		t.Error("GetLatestRound() returned round with zero timestamp")
	}
}

func TestDrandBeacon_StartStop(t *testing.T) {
	beacon, err := NewDrandBeacon("")
	if err != nil {
		t.Fatalf("Failed to create beacon: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the beacon with a short interval
	err = beacon.Start(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for at least one round
	select {
	case round := <-beacon.Rounds():
		if round == nil {
			t.Error("Received nil round")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for round")
	}

	// Stop the beacon
	err = beacon.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify no more rounds are received
	select {
	case round := <-beacon.Rounds():
		t.Errorf("Received unexpected round after stop: %v", round)
	case <-time.After(1500 * time.Millisecond):
		// Expected timeout
	}
}
