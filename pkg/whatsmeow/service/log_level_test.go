package whatsmeow_service

import "testing"

func TestWhatsmeowLogLevel(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"false":   "",
		"true":    "",
		"1":       "",
		"debug":   "DEBUG",
		" Info ":  "INFO",
		"WARN":    "WARN",
		"error":   "ERROR",
		"verbose": "",
	}
	for raw, want := range cases {
		if got := whatsmeowLogLevel(raw); got != want {
			t.Errorf("whatsmeowLogLevel(%q) = %q, want %q", raw, got, want)
		}
	}
}
