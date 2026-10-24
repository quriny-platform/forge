package forge

import (
	"errors"
	"fmt"
)

// ErrNoRows indicates a query succeeded but returned zero rows. Only
// exactly-one-row helpers return it; Query yields empty Rows instead.
// Callers compare with errors.Is, since wrapping layers add context.
var ErrNoRows = errors.New("forge: no rows in result set")

// ErrTooManyRows indicates a query succeeded but returned more than one row
// to an exactly-one-row helper. The connection stays usable; remaining rows
// are discarded. Callers compare with errors.Is, since wrapping layers add
// context.
var ErrTooManyRows = errors.New("forge: too many rows in result set")

// ErrClosed indicates a Conn, Stmt, Tx, or pooled resource was used after
// Close. The condition is permanent, not retryable: callers must obtain a new
// connection. Callers compare with errors.Is, since wrapping layers add
// context.
var ErrClosed = errors.New("forge: connection is closed")

// QueryError carries a failed query with its cause. Err holds the underlying
// failure, which may be one of this package's sentinels; callers reach it
// with errors.Is or errors.As.
//
// Args is retained for programmatic inspection only. Neither Error nor
// Redacted renders it, so an argument value cannot reach a log line through
// this type.
type QueryError struct {
	SQL  string
	Args []any
	Err  error
}

// Error reports the failing SQL and its cause, omitting the Args.
func (q *QueryError) Error() string {
	if q.Err == nil {
		return fmt.Sprintf("forge: query %q", q.SQL)
	}
	return fmt.Sprintf("forge: query %q: %s", q.SQL, q.Err)
}

// Unwrap returns the underlying cause, making QueryError transparent to
// errors.Is and errors.As.
func (q *QueryError) Unwrap() error {
	return q.Err
}

// Redacted is the form loggers should use. It reports the SQL, how many
// arguments were bound, and the cause, but never an argument value.
func (q *QueryError) Redacted() string {
	if q.Err == nil {
		return fmt.Sprintf("forge: query %q (%d args redacted)", q.SQL, len(q.Args))
	}
	return fmt.Sprintf("forge: query %q (%d args redacted): %s", q.SQL, len(q.Args), q.Err)
}
