package forge

import (
	"context"
	"testing"
)

// Compile-time contract checks: if an interface changes shape, these lines
// stop compiling before any behavioural test runs.
var (
	_ Driver = (*fakeDriver)(nil)
	_ Conn   = (*fakeConn)(nil)
	_ Rows   = (*fakeRows)(nil)
	_ Result = (*fakeResult)(nil)
	_ Stmt   = (*fakeStmt)(nil)
	_ Tx     = (*fakeTx)(nil)
)

type fakeDriver struct{}

func (*fakeDriver) Open(context.Context, string) (Conn, error) {
	return &fakeConn{}, nil
}

type fakeConn struct{}

func (*fakeConn) Ping(context.Context) error { return nil }

func (*fakeConn) Exec(context.Context, string, ...any) (Result, error) {
	return &fakeResult{}, nil
}

func (*fakeConn) Query(context.Context, string, ...any) (Rows, error) {
	return newFakeRows(), nil
}

func (*fakeConn) Prepare(context.Context, string, string) (Stmt, error) {
	return &fakeStmt{}, nil
}

func (*fakeConn) Begin(context.Context, *TxOptions) (Tx, error) {
	return &fakeTx{}, nil
}

func (*fakeConn) Close(context.Context) error { return nil }

type fakeRows struct {
	data   [][]any
	pos    int // index of the current row; -1 before the first Next
	closed bool
}

func newFakeRows(data ...[]any) *fakeRows {
	return &fakeRows{data: data, pos: -1}
}

func (r *fakeRows) Next() bool {
	if r.closed || r.pos+1 >= len(r.data) {
		return false
	}
	r.pos++
	return true
}

func (*fakeRows) Scan(...any) error { return nil }

func (r *fakeRows) Values() ([]any, error) {
	if r.pos < 0 || r.pos >= len(r.data) {
		return nil, ErrNoRows
	}
	return r.data[r.pos], nil
}

func (*fakeRows) FieldDescriptions() []FieldDescription {
	return []FieldDescription{{Name: "id", DataTypeOID: 23}}
}

func (*fakeRows) Err() error { return nil }

// Close is idempotent: a second call does nothing and reports no error.
func (r *fakeRows) Close() error {
	r.closed = true
	return nil
}

type fakeResult struct{}

func (*fakeResult) RowsAffected() int64 { return 0 }

func (*fakeResult) Command() string { return "SELECT 0" }

type fakeStmt struct{}

func (*fakeStmt) Exec(context.Context, ...any) (Result, error) {
	return &fakeResult{}, nil
}

func (*fakeStmt) Query(context.Context, ...any) (Rows, error) {
	return newFakeRows(), nil
}

func (*fakeStmt) Close(context.Context) error { return nil }

type fakeTx struct{}

func (*fakeTx) Exec(context.Context, string, ...any) (Result, error) {
	return &fakeResult{}, nil
}

func (*fakeTx) Query(context.Context, string, ...any) (Rows, error) {
	return newFakeRows(), nil
}

func (*fakeTx) Commit(context.Context) error { return nil }

func (*fakeTx) Rollback(context.Context) error { return nil }

func TestRowsIteration(t *testing.T) {
	rows := newFakeRows([]any{1}, []any{2})

	var seen int
	for rows.Next() {
		seen++
		if _, err := rows.Values(); err != nil {
			t.Fatalf("Values() on row %d: %v", seen, err)
		}
	}
	if seen != 2 {
		t.Errorf("Next() true %d times, want 2", seen)
	}
	if rows.Next() {
		t.Error("Next() after exhaustion = true, want false")
	}
	if err := rows.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}

	for i := range 2 {
		if err := rows.Close(); err != nil {
			t.Errorf("Close() call %d = %v, want nil", i+1, err)
		}
	}

	if rows.Next() {
		t.Error("Next() after Close = true, want false")
	}
}
