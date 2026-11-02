package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"

	"admin-backend/internal/modules"
)

// --- Driver SQL falso (stdlib only, sin dependencias nuevas) ----------------
//
// Registra las consultas y sentencias ejecutadas para que los tests de
// despacho/transacción/backcompat puedan afirmar la forma del SQL y el uso (o
// no) de transacción sin una base de datos real.

type recordedDriver struct {
	mu        sync.Mutex
	execs     []string // sentencias de ExecContext
	queries   []string // consultas de QueryContext
	began     int      // nº de BEGIN (transacciones abiertas)
	committed int
	rolled    int
}

func (d *recordedDriver) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.execs = nil
	d.queries = nil
	d.began = 0
	d.committed = 0
	d.rolled = 0
}

func (d *recordedDriver) Open(string) (driver.Conn, error) { return &recordedConn{drv: d}, nil }

type recordedConn struct{ drv *recordedDriver }

func (c *recordedConn) Prepare(query string) (driver.Stmt, error) {
	return &recordedStmt{drv: c.drv, query: query}, nil
}
func (c *recordedConn) Close() error { return nil }
func (c *recordedConn) Begin() (driver.Tx, error) {
	c.drv.mu.Lock()
	c.drv.began++
	c.drv.mu.Unlock()
	return &recordedTx{drv: c.drv}, nil
}

// ExecerContext / QueryerContext permiten capturar la consulta cruda.
func (c *recordedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.drv.mu.Lock()
	c.drv.execs = append(c.drv.execs, query)
	c.drv.mu.Unlock()
	return driver.RowsAffected(1), nil
}

func (c *recordedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.drv.mu.Lock()
	c.drv.queries = append(c.drv.queries, query)
	c.drv.mu.Unlock()
	if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "SELECT COUNT") {
		return &countRows{}, nil
	}
	return &emptyRows{}, nil
}

type recordedTx struct{ drv *recordedDriver }

func (t *recordedTx) Commit() error {
	t.drv.mu.Lock()
	t.drv.committed++
	t.drv.mu.Unlock()
	return nil
}
func (t *recordedTx) Rollback() error {
	t.drv.mu.Lock()
	t.drv.rolled++
	t.drv.mu.Unlock()
	return nil
}

type recordedStmt struct {
	drv   *recordedDriver
	query string
}

func (s *recordedStmt) Close() error  { return nil }
func (s *recordedStmt) NumInput() int { return -1 }
func (s *recordedStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	s.drv.execs = append(s.drv.execs, s.query)
	s.drv.mu.Unlock()
	return driver.RowsAffected(1), nil
}
func (s *recordedStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.drv.mu.Lock()
	s.drv.queries = append(s.drv.queries, s.query)
	s.drv.mu.Unlock()
	return &emptyRows{}, nil
}

// emptyRows: SELECT de datos que no devuelve filas (una sola columna arbitraria).
type emptyRows struct{ done bool }

func (r *emptyRows) Columns() []string { return []string{"c"} }
func (r *emptyRows) Close() error      { return nil }
func (r *emptyRows) Next(dest []driver.Value) error {
	return io.EOF
}

// countRows: devuelve una sola fila con un count de 0.
type countRows struct{ done bool }

func (r *countRows) Columns() []string { return []string{"count"} }
func (r *countRows) Close() error      { return nil }
func (r *countRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(0)
	return nil
}

// newRecordedExecutor construye un PGExecutor que abre siempre el driver falso y
// cuyo SecretProvider devuelve un DSN ficticio no vacío.
func newRecordedExecutor(drv *recordedDriver) *PGExecutor {
	opener := func(dsn string) (*sql.DB, error) {
		return sql.OpenDB(fakeConnector{drv: drv}), nil
	}
	return NewPGExecutor(fakeSecrets{}, opener)
}

type fakeConnector struct{ drv *recordedDriver }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &recordedConn{drv: c.drv}, nil
}
func (c fakeConnector) Driver() driver.Driver { return c.drv }

type fakeSecrets struct{}

func (fakeSecrets) RevealSecrets(context.Context, string) (modules.Secrets, error) {
	return modules.Secrets{DSN: "postgres://fake"}, nil
}
