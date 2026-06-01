package main

import (
	"testing"
	"time"

	"websudo/internal/config"
)

func TestApprovalTimeout(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{name: "positive", seconds: 42, want: 42 * time.Second},
		{name: "zero", seconds: 0, want: 10 * time.Minute},
		{name: "negative", seconds: -1, want: 10 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := approvalTimeout(config.Config{ApprovalTimeoutSeconds: tc.seconds})
			if got != tc.want {
				t.Fatalf("approvalTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}
