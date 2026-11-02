package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/engine"
	"admin-backend/internal/modules"
)

// --- Dobles para el test de ExecActionList ----------------------------------

// execListFakeRepo implementa modules.Repository devolviendo una definición fija
// por slug. Solo FindBySlug se ejercita en estas pruebas.
type execListFakeRepo struct {
	def *modules.Definition
	err error
}

func (r *execListFakeRepo) List(context.Context) ([]modules.Definition, error) { return nil, nil }
func (r *execListFakeRepo) FindBySlug(context.Context, string) (*modules.Definition, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.def, nil
}
func (r *execListFakeRepo) Create(context.Context, *modules.Definition, modules.Secrets) error {
	return nil
}
func (r *execListFakeRepo) Update(context.Context, *modules.Definition, modules.Secrets) error {
	return nil
}
func (r *execListFakeRepo) Delete(context.Context, string) error { return nil }

// execListSecrets provee un DSN ficticio no vacío para que el PGExecutor abra el
// pool del driver falso.
type execListSecrets struct{}

func (execListSecrets) RevealSecrets(context.Context, string) (modules.Secrets, error) {
	return modules.Secrets{DSN: "postgres://fake"}, nil
}

// execListDriver registra las consultas ejecutadas para afirmar que el WHERE del
// filtro de acción llegó al SELECT de datos. El COUNT devuelve 0 filas; el
// SELECT de datos no devuelve filas.
type execListDriver struct {
	mu      sync.Mutex
	queries []string
}

func (d *execListDriver) record(q string) {
	d.mu.Lock()
	d.queries = append(d.queries, q)
	d.mu.Unlock()
}
func (d *execListDriver) all() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.queries...)
}

func (d *execListDriver) Open(string) (driver.Conn, error) { return &execListConn{drv: d}, nil }

type execListConn struct{ drv *execListDriver }

func (c *execListConn) Prepare(string) (driver.Stmt, error) { return nil, io.EOF }
func (c *execListConn) Close() error                        { return nil }
func (c *execListConn) Begin() (driver.Tx, error)           { return nil, io.EOF }
func (c *execListConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.drv.record(query)
	if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "SELECT COUNT") {
		return &execListCountRows{}, nil
	}
	return &execListEmptyRows{}, nil
}

type execListEmptyRows struct{}

func (r *execListEmptyRows) Columns() []string              { return []string{"c"} }
func (r *execListEmptyRows) Close() error                   { return nil }
func (r *execListEmptyRows) Next(dest []driver.Value) error { return io.EOF }

type execListCountRows struct{ done bool }

func (r *execListCountRows) Columns() []string { return []string{"count"} }
func (r *execListCountRows) Close() error      { return nil }
func (r *execListCountRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(0)
	return nil
}

type execListConnector struct{ drv *execListDriver }

func (c execListConnector) Connect(context.Context) (driver.Conn, error) {
	return &execListConn{drv: c.drv}, nil
}
func (c execListConnector) Driver() driver.Driver { return c.drv }

// newExecListPG construye un *engine.PGExecutor que abre siempre el driver falso.
func newExecListPG(drv *execListDriver) *engine.PGExecutor {
	opener := func(string) (*sql.DB, error) {
		return sql.OpenDB(execListConnector{drv: drv}), nil
	}
	return engine.NewPGExecutor(execListSecrets{}, opener)
}

// selectFilterDef arma una definición Postgres N=1 con una acción SELECT que
// filtra por "estado" (exact).
func selectFilterDef() *modules.Definition {
	return &modules.Definition{
		Slug:         "medios",
		Name:         "Medios de pago",
		SourceType:   modules.SourcePostgres,
		FetchMapping: map[string]any{"table": "medios_pago"},
		DisplayMapping: map[string]any{"columns": []any{
			map[string]any{"field": "id", "visible": true},
			map[string]any{"field": "estado", "visible": true},
		}},
		DataSource: modules.DataSource{PGWhitelist: map[string][]string{
			"medios_pago": {"id", "estado"},
		}},
		Actions: []modules.Action{
			{ActionKey: "list", Kind: modules.ActionKindList, SQLOp: "SELECT"},
			{
				ActionKey:   "custom:buscar",
				Kind:        modules.ActionKindCustom,
				SQLOp:       "SELECT",
				InputFields: []modules.FieldSpec{{Name: "estado", Type: "string", MatchType: modules.SearchMatchExact}},
			},
		},
	}
}

// Verifica que ExecActionList lee la paginación de la query y el input del
// cuerpo JSON, valida los campos como OPCIONALES e invoca ListFiltered (el WHERE
// del filtro llega al SELECT de datos), devolviendo el envoltorio ok/data.
func TestExecActionList_FiltersFeedList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	drv := &execListDriver{}
	h := NewModulesHandler(&execListFakeRepo{def: selectFilterDef()}, newExecListPG(drv), nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: "medios"}, {Key: "actionId", Value: "custom:buscar"}}
	body := strings.NewReader(`{"estado":"ACTIVO"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/medios/actions/custom:buscar/list?page=2&records_per_page=10", body)
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecActionList(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// El filtro exact sobre "estado" debe aparecer en el SELECT de datos.
	var dataQuery string
	for _, q := range drv.all() {
		if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(q)), "SELECT COUNT") {
			continue
		}
		dataQuery = q
	}
	if dataQuery == "" {
		t.Fatalf("no se registró el SELECT de datos; queries=%v", drv.all())
	}
	if !strings.Contains(dataQuery, `lower(btrim("estado"::text)) =`) {
		t.Fatalf("el WHERE del filtro no llegó al SELECT de datos: %s", dataQuery)
	}

	// La respuesta tiene la MISMA forma que List (ok/data/{items,pagination}).
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Items      []map[string]any `json:"items"`
			Pagination struct {
				Page           int `json:"page"`
				RecordsPerPage int `json:"recordsPerPage"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	if !resp.OK {
		t.Fatalf("ok debe ser true: %s", rec.Body.String())
	}
	// La paginación proviene de la query (page=2, per=10).
	if resp.Data.Pagination.Page != 2 || resp.Data.Pagination.RecordsPerPage != 10 {
		t.Fatalf("paginación inesperada: %+v", resp.Data.Pagination)
	}
}

// searchFilterDef arma una definición Postgres N=1 con una acción de Kind
// "search" (lectura con filtro, hermana de 'list') que filtra por "estado"
// (exact). Sirve para confirmar que el runtime de lectura no distingue Kind:
// se comporta igual que una custom+SELECT.
func searchFilterDef() *modules.Definition {
	return &modules.Definition{
		Slug:         "medios",
		Name:         "Medios de pago",
		SourceType:   modules.SourcePostgres,
		FetchMapping: map[string]any{"table": "medios_pago"},
		DisplayMapping: map[string]any{"columns": []any{
			map[string]any{"field": "id", "visible": true},
			map[string]any{"field": "estado", "visible": true},
		}},
		DataSource: modules.DataSource{PGWhitelist: map[string][]string{
			"medios_pago": {"id", "estado"},
		}},
		Actions: []modules.Action{
			{ActionKey: "list", Kind: modules.ActionKindList, SQLOp: "SELECT"},
			{
				ActionKey:   "buscar",
				Kind:        modules.ActionKindSearch,
				SQLOp:       "SELECT",
				InputFields: []modules.FieldSpec{{Name: "estado", Type: "string", MatchType: modules.SearchMatchExact}},
			},
		},
	}
}

// Verifica que una acción de Kind "search" con SQLOp SELECT y un InputField
// lleno aplica el WHERE del campo en ListFiltered EXACTAMENTE igual que una
// custom+SELECT: el runtime de lectura no distingue por Kind.
func TestExecActionList_SearchKindFiltersFeedList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	drv := &execListDriver{}
	h := NewModulesHandler(&execListFakeRepo{def: searchFilterDef()}, newExecListPG(drv), nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: "medios"}, {Key: "actionId", Value: "buscar"}}
	body := strings.NewReader(`{"estado":"ACTIVO"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/medios/actions/buscar/list?page=2&records_per_page=10", body)
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecActionList(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// El filtro exact sobre "estado" debe aparecer en el SELECT de datos.
	var dataQuery string
	for _, q := range drv.all() {
		if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(q)), "SELECT COUNT") {
			continue
		}
		dataQuery = q
	}
	if dataQuery == "" {
		t.Fatalf("no se registró el SELECT de datos; queries=%v", drv.all())
	}
	if !strings.Contains(dataQuery, `lower(btrim("estado"::text)) =`) {
		t.Fatalf("el WHERE del filtro no llegó al SELECT de datos: %s", dataQuery)
	}
}

// Verifica que un filtro vacío NO bloquea (campos opcionales) y se omite del
// WHERE: el SELECT de datos no debe traer el predicado del filtro.
func TestExecActionList_EmptyFilterNotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	drv := &execListDriver{}
	h := NewModulesHandler(&execListFakeRepo{def: selectFilterDef()}, newExecListPG(drv), nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: "medios"}, {Key: "actionId", Value: "custom:buscar"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/medios/actions/custom:buscar/list", strings.NewReader(`{"estado":"   "}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecActionList(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	for _, q := range drv.all() {
		if strings.Contains(q, `"estado"::text)) =`) {
			t.Fatalf("un filtro vacío no debe producir predicado: %s", q)
		}
	}
}

// Verifica el 404 cuando la acción no existe en el módulo.
func TestExecActionList_ActionNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModulesHandler(&execListFakeRepo{def: selectFilterDef()}, newExecListPG(&execListDriver{}), nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: "medios"}, {Key: "actionId", Value: "no-existe"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/medios/actions/no-existe/list", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecActionList(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
