package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

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
