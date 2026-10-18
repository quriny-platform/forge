package forge

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
	}{
		{"ErrNoRows", ErrNoRows},
		{"ErrTooManyRows", ErrTooManyRows},
		{"ErrClosed", ErrClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.sentinel, tt.sentinel) {
				t.Fatalf("errors.Is(%v, itself) = false, want true", tt.sentinel)
			}

			wrapped := fmt.Errorf("postgres: query %q: %w", "SELECT 1", tt.sentinel)
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(wrapped, %v) = false, want true", tt.sentinel)
			}
		})
	}
}

func TestSentinelErrorsDistinct(t *testing.T) {
	if errors.Is(ErrNoRows, ErrTooManyRows) {
		t.Error("ErrNoRows must not match ErrTooManyRows")
	}
	if errors.Is(ErrNoRows, ErrClosed) {
		t.Error("ErrNoRows must not match ErrClosed")
	}
}
