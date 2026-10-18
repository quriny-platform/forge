package forge

import "errors"

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
