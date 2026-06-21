package networkmanager

import (
	"testing"
	"time"
)

func TestTunnelStale(t *testing.T) {
	now := time.Unix(10000, 0)
	threshold := 60 * time.Second

	tests := []struct {
		name          string
		deviceErr     bool
		lastHandshake time.Time
		want          bool
	}{
		{"device error", true, now, true},
		{"never handshook", false, time.Time{}, true},
		{"fresh handshake", false, now.Add(-10 * time.Second), false},
		{"at threshold", false, now.Add(-60 * time.Second), false},
		{"stale handshake", false, now.Add(-120 * time.Second), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TunnelStale(tt.deviceErr, tt.lastHandshake, now, threshold)
			if got != tt.want {
				t.Errorf("TunnelStale(deviceErr=%v, age=%v) = %v, want %v",
					tt.deviceErr, now.Sub(tt.lastHandshake), got, tt.want)
			}
		})
	}
}
