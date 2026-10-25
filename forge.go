package forge

import "context"

// Driver opens connections to a database engine. Implementations are
// stateless factories: all per-connection state lives in the returned Conn.
type Driver interface {
	// Open connects using dsn and returns a ready Conn. It honors ctx for
	// the whole handshake, including authentication.
	Open(ctx context.Context, dsn string) (Conn, error)
}

// Conn is a single database connection. It is not safe for concurrent use;
// share connections through a pool.
//
// Every query entry point takes the SQL text and its arguments separately.
// Values are never interpolated into sql by this package.
type Conn interface {
	// Ping verifies the connection is alive.
	Ping(ctx context.Context) error
	// Exec runs a statement that returns no rows.
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	// Query runs a statement and returns its rows. The caller must Close them.
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	// Prepare parses sql once under name for repeated execution.
	Prepare(ctx context.Context, name, sql string) (Stmt, error)
	// Begin starts a transaction. A nil opts means server defaults.
	Begin(ctx context.Context, opts *TxOptions) (Tx, error)
	// Close ends the connection. Later use returns ErrClosed.
	Close(ctx context.Context) error
}

// Rows iterates over a query result. Next reports whether another row is
// available; an error that ended iteration is reported by Err, the same
// contract as database/sql.Rows and bufio.Scanner.
type Rows interface {
	// Next advances to the next row. It returns false when rows are
	// exhausted, after Close, or on error.
	Next() bool
	// Scan copies the current row's columns into dest.
	Scan(dest ...any) error
	// Values returns the current row's columns decoded to Go values.
	Values() ([]any, error)
	// FieldDescriptions describes the result's columns.
	FieldDescriptions() []FieldDescription
	// Err returns the error, if any, that ended iteration.
	Err() error
	// Close releases the rows. It is safe to call more than once.
	Close() error
}

// Result summarizes a statement that returns no rows.
type Result interface {
	// RowsAffected is the number of rows changed by the statement.
	RowsAffected() int64
	// Command is the server's command tag, such as "INSERT 0 1".
	Command() string
}

// Stmt is a prepared statement bound to the Conn that created it.
type Stmt interface {
	Exec(ctx context.Context, args ...any) (Result, error)
	Query(ctx context.Context, args ...any) (Rows, error)
	Close(ctx context.Context) error
}

// Tx is an open transaction. Exactly one of Commit or Rollback ends it.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// FieldDescription describes one column of a result set.
type FieldDescription struct {
	Name            string
	TableOID        uint32
	AttributeNumber uint16
	DataTypeOID     uint32
	DataTypeSize    int16
	TypeModifier    int32
	Format          int16
}

// TxOptions configures Begin. The zero value selects the server defaults.
type TxOptions struct {
	IsoLevel       IsoLevel
	AccessMode     AccessMode
	DeferrableMode DeferrableMode
}

// IsoLevel is a transaction isolation level.
type IsoLevel string

// Isolation levels accepted in TxOptions.
const (
	Serializable    IsoLevel = "serializable"
	RepeatableRead  IsoLevel = "repeatable read"
	ReadCommitted   IsoLevel = "read committed"
	ReadUncommitted IsoLevel = "read uncommitted"
)

// AccessMode is a transaction access mode.
type AccessMode string

// Access modes accepted in TxOptions.
const (
	ReadWrite AccessMode = "read write"
	ReadOnly  AccessMode = "read only"
)

// DeferrableMode is a transaction deferrability setting.
type DeferrableMode string

// Deferrability settings accepted in TxOptions.
const (
	Deferrable    DeferrableMode = "deferrable"
	NotDeferrable DeferrableMode = "not deferrable"
)
