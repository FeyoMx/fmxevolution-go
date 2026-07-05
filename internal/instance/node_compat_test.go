package instance

import (
	"testing"
)

func TestEvolutionConnectionStatus(t *testing.T) {
	cases := map[string]string{
		"open":         "open",
		"connected":    "open",
		"connecting":   "connecting",
		"qr_pending":   "connecting",
		"reconnecting": "connecting",
		"close":        "close",
		"disconnected": "close",
		"created":      "close",
		"":             "close",
	}
	for in, want := range cases {
		if got := evolutionConnectionStatus(in); got != want {
			t.Errorf("evolutionConnectionStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
