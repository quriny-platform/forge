package forge

import "errors"

var (
	ErrNoRows      = errors.New("forge: no rows in result set")
	ErrTooManyRows = errors.New("forge: too many rows in result set")
	ErrClosed      = errors.New("forge: connection is closed")
)
