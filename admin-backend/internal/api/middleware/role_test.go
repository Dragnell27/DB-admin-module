package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/users"
)

// buildRoleRouter arma un router con el middleware de sesión seguido del de rol
// sobre una ruta de escritura, replicando el montaje que hará el router real
// (tarea 9.1): primero SessionAuth deja la identidad en el contexto y luego
// RequireAdmin la autoriza.
func buildRoleRouter() *gin.Engine {
	r := gin.New()
	r.POST("/write", SessionAuth(testSecret), RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func doWriteRequest(r *gin.Engine, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireAdminAllowsAdminRole(t *testing.T) {
	r := buildRoleRouter()
	token := newToken(t, "id", "admin-user", users.RoleAdmin, 60)

	w := doWriteRequest(r, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 para rol admin, se obtuvo %d (body=%q)", w.Code, w.Body.String())
	}
}

func TestRequireAdminRejectsViewerWith403(t *testing.T) {
	r := buildRoleRouter()
	token := newToken(t, "id", "viewer-user", users.RoleViewer, 60)

	w := doWriteRequest(r, "Bearer "+token)

	if w.Code != http.StatusForbidden {
		t.Fatalf("se esperaba 403 para rol viewer, se obtuvo %d (body=%q)", w.Code, w.Body.String())
	}
	body := decodeError(t, w)
	if body.OK {
		t.Error("el envoltorio de error debe tener ok=false")
	}
	if body.ErrorCode != "AUTH_FORBIDDEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_FORBIDDEN", body.ErrorCode)
	}
}

// TestRequireAdminWithoutRoleInContext verifica que, si por error se montara
// RequireAdmin sin sesión previa (sin rol en el contexto), responde 403 en vez
// de permitir el paso.
func TestRequireAdminWithoutRoleInContext(t *testing.T) {
	r := gin.New()
	r.POST("/write", RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doWriteRequest(r, "")

	if w.Code != http.StatusForbidden {
		t.Fatalf("se esperaba 403 sin rol en el contexto, se obtuvo %d", w.Code)
	}
	if ec := decodeError(t, w).ErrorCode; ec != "AUTH_FORBIDDEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_FORBIDDEN", ec)
	}
}

// TestRequireAdminMissingTokenReturns401 confirma la división de
// responsabilidades: la ausencia de token la gestiona el middleware de sesión
// con 401, antes de llegar a RequireAdmin.
func TestRequireAdminMissingTokenReturns401(t *testing.T) {
	r := buildRoleRouter()

	w := doWriteRequest(r, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 sin token (lo gestiona el middleware de sesión), se obtuvo %d", w.Code)
	}
}
