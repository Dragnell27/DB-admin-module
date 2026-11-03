package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/engine"
	"admin-backend/internal/modules"
)

// deriveRepo es un Repository mínimo que devuelve una definición fija con una
// acción INSERT con estrategias; el resto de métodos no se usan en el test.
type deriveRepo struct{ def *modules.Definition }

func (r deriveRepo) List(ctx context.Context) ([]modules.Definition, error) { return nil, nil }
func (r deriveRepo) FindBySlug(ctx context.Context, slug string) (*modules.Definition, error) {
	return r.def, nil
}
func (r deriveRepo) Create(ctx context.Context, def *modules.Definition, sec modules.Secrets) error {
	return nil
}
func (r deriveRepo) Update(ctx context.Context, def *modules.Definition, sec modules.Secrets) error {
	return nil
}
func (r deriveRepo) Delete(ctx context.Context, slug string) error { return nil }

// deriveExec es un Executor cuyo Exec devuelve engine.ErrDeriveRelation.
type deriveExec struct{}

func (deriveExec) List(ctx context.Context, def modules.Definition, country string, q engine.ListQuery) ([]map[string]any, engine.Pagination, error) {
	return nil, engine.Pagination{}, nil
}
func (deriveExec) Exec(ctx context.Context, def modules.Definition, country string, act modules.Action, input map[string]any) (any, string, error) {
	return nil, "", engine.ErrDeriveRelation
}

// ExecAction mapea engine.ErrDeriveRelation a 422 E422_DERIVE con un mensaje
// FIJO de negocio (sin concatenar el err.Error() interno).
func TestExecAction_DeriveRelationMapped(t *testing.T) {
	gin.SetMode(gin.TestMode)

	def := &modules.Definition{
		Slug:       "activar-medio-pago",
		SourceType: modules.SourcePostgres,
		Actions: []modules.Action{
			{
				ActionKey:   "activar",
				Kind:        modules.ActionKindCreate,
				SQLOp:       "INSERT",
				TargetTable: "gt_valores_hijo",
				InputFields: []modules.FieldSpec{
					{Name: "valor_lista", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
						Kind: modules.StrategyCaptureFromCatalog,
					}},
				},
			},
		},
	}

	h := NewModulesHandler(deriveRepo{def: def}, deriveExec{}, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: def.Slug}, {Key: "actionId", Value: "activar"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/activar-medio-pago/actions/activar",
		strings.NewReader(`{"valor_lista": 55}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecAction(c)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want 422 (body=%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	if body.ErrorCode != "E422_DERIVE" {
		t.Fatalf("code: got %q want E422_DERIVE (body=%s)", body.ErrorCode, rec.Body.String())
	}
	// Mensaje fijo: no contiene el texto interno del error del motor.
	if strings.Contains(body.Message, "no se pudo resolver la relación") && strings.Contains(body.Message, "engine:") {
		t.Fatalf("el mensaje no debe filtrar el err.Error() interno: %q", body.Message)
	}
	if body.Message == "" {
		t.Fatalf("se esperaba un mensaje de negocio fijo")
	}
}
