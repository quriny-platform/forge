package forge

import (
	"errors"
	"fmt"
	"strings"
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

func TestQueryError(t *testing.T) {
	tests := []struct {
		name string
		err  *QueryError
		want string
	}{
		{
			name: "nil cause",
			err:  &QueryError{SQL: "SELECT 1"},
			want: `forge: query "SELECT 1"`,
		},
		{
			name: "wraps sentinel",
			err:  &QueryError{SQL: "SELECT 1", Err: ErrNoRows},
			want: `forge: query "SELECT 1": forge: no rows in result set`,
		},
		{
			name: "quotes embedded quote",
			err:  &QueryError{SQL: `SELECT "id"`, Err: ErrClosed},
			want: `forge: query "SELECT \"id\"": forge: connection is closed`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueryErrorIs(t *testing.T) {
	err := &QueryError{SQL: "SELECT 1", Err: ErrNoRows}

	if !errors.Is(err, ErrNoRows) {
		t.Error("errors.Is(err, ErrNoRows) = false, want true")
	}
	if errors.Is(err, ErrClosed) {
		t.Error("errors.Is(err, ErrClosed) = true, want false")
	}
}

func TestQueryErrorAs(t *testing.T) {
	statement := "SELECT 1"
	var err error = &QueryError{SQL: statement, Err: ErrNoRows}
	for i := range 3 {
		err = fmt.Errorf("layer %d: %w", i, err)
	}

	var qe *QueryError
	if !errors.As(err, &qe) {
		t.Fatalf("errors.As(err, &qe) = false, want true")
	}
	if qe.SQL != statement {
		t.Errorf("qe.SQL = %q, want %q", qe.SQL, statement)
	}
	if !errors.Is(err, ErrNoRows) {
		t.Error("errors.Is through 3 layers = false, want true")
	}
}

func TestQueryErrorRedactsArgs(t *testing.T) {
	err := &QueryError{
		SQL:  "SELECT * FROM users WHERE password = $1",
		Args: []any{"hunter2"},
		Err:  ErrNoRows,
	}

	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("Error() leaked an argument value: %q", err.Error())
	}
	if len(err.Args) != 1 {
		t.Errorf("len(Args) = %d, want 1; args stay for inspection", len(err.Args))
	}
}

func FuzzQueryErrorRedaction(f *testing.F) {
	f.Add("hunter2")
	f.Add("")
	f.Add("'; DROP TABLE users;--")
	f.Add("%s %q %v %d")
	f.Add("\x00\xff")

	f.Fuzz(func(t *testing.T, secret string) {
		// The marker makes the needle unique, so a fuzz input like "forge"
		// or "" cannot match the fixed text of the message by accident.
		needle := "ARG<" + secret + ">"

		err := &QueryError{
			SQL:  "SELECT * FROM users WHERE a = $1 AND b = $2",
			Args: []any{needle, []byte(needle)},
			Err:  ErrNoRows,
		}

		if strings.Contains(err.Error(), needle) {
			t.Errorf("Error() leaked arg: %q", err.Error())
		}
		if strings.Contains(err.Redacted(), needle) {
			t.Errorf("Redacted() leaked arg: %q", err.Redacted())
		}
	})
}
