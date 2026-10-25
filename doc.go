// Package forge defines the engine-agnostic core of the forge database
// toolkit: the Driver, Conn, Rows, Result, Stmt and Tx interfaces, plus the
// error types they share. It is modeled on database/sql/driver and imports
// only the standard library.
//
// # Layers
//
// forge sits at the bottom of the module. Engine implementations live in
// postgres/..., which adapts the wire protocol to these interfaces. The pool
// and scan packages build on this package alone and never import an engine.
// Imports point downward only; internal/architecture_test.go enforces this.
//
// # Queries
//
// Every query entry point takes the SQL text and its arguments separately:
// (sql string, args ...any). No function in this package accepts SQL built
// from values, and there is no Queryf. Values travel as bound parameters, so
// injection cannot be expressed through this API.
//
// # Errors
//
// Callers compare against the sentinels with errors.Is, since wrapping layers
// add context. ErrNoRows and ErrTooManyRows come from exactly-one-row helpers.
// ErrClosed marks use of a closed Conn, Stmt or Tx and is permanent.
//
// A failed query is reported as a *QueryError. It keeps the arguments for
// inspection in code, but neither Error nor Redacted ever prints them.
package forge
