package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/engine"
	"admin-backend/internal/modules"
)

// Verifica que un error que envuelve modules.ErrMultiTableValidation se traduce
// a HTTP 422 con code E422_VALIDATION y conserva el mensaje español específico,
// en vez de caer al E500 genérico (Hallazgo 1 — cableado de error).
func TestWriteDefError_MultiTableValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	h := &ModulesHandler{}
	msg := "el alias de salida \"tabla\".\"columna\" excede 63 bytes"
	err := fmt.Errorf("%w: %s", modules.ErrMultiTableValidation, msg)
	h.writeDefError(c, err)

	if rec.Code != 422 {
		t.Fatalf("status: got %d want 422", rec.Code)
	}
	var body struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	if body.ErrorCode != "E422_VALIDATION" {
		t.Fatalf("code: got %q want E422_VALIDATION (body=%s)", body.ErrorCode, rec.Body.String())
	}
	if body.Message == "" {
		t.Fatalf("se esperaba el mensaje específico, body=%s", rec.Body.String())
	}
}

// uniquenessExec es un Executor cuyo Exec devuelve engine.ErrUniqueness (simula
// que el pre-check o el 23505 detectaron un duplicado).
type uniquenessExec struct{}

func (uniquenessExec) List(ctx context.Context, def modules.Definition, country string, q engine.ListQuery) ([]map[string]any, engine.Pagination, error) {
	return nil, engine.Pagination{}, nil
}
func (uniquenessExec) Exec(ctx context.Context, def modules.Definition, country string, act modules.Action, input map[string]any) (any, string, error) {
	return nil, "", fmt.Errorf("%w: contexto interno", engine.ErrUniqueness)
}

// ExecAction mapea engine.ErrUniqueness a 409 E409_UNIQUENESS mostrando el
// mensaje de NEGOCIO del admin (Uniqueness.Message), nunca el SQL ni el texto
// interno del error del motor (FEAT-002, design.md §8).
func TestExecAction_UniquenessMapped(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const businessMsg = "Ese medio de pago ya está activo para el comercio"
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
					{Name: "valor_lista", Source: modules.FieldSourceInput},
				},
				Uniqueness: &modules.UniquenessCheck{
					Columns: []string{"valor_lista"},
					Message: businessMsg,
				},
			},
		},
	}

	h := NewModulesHandler(deriveRepo{def: def}, uniquenessExec{}, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "slug", Value: def.Slug}, {Key: "actionId", Value: "activar"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/modules/activar-medio-pago/actions/activar",
		strings.NewReader(`{"valor_lista": "MP01"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ExecAction(c)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	if body.ErrorCode != "E409_UNIQUENESS" {
		t.Fatalf("code: got %q want E409_UNIQUENESS (body=%s)", body.ErrorCode, rec.Body.String())
	}
	if body.Message != businessMsg {
		t.Fatalf("mensaje: got %q want el mensaje de negocio del admin", body.Message)
	}
	if strings.Contains(body.Message, "engine:") {
		t.Fatalf("el mensaje no debe filtrar el err.Error() interno: %q", body.Message)
	}
}
