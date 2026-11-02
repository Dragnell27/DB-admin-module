// Feature: panel-admin-ott, Property 14: El rol determina el acceso a operaciones de escritura
//
// Prueba basada en propiedades (PBT) del control de acceso por rol del BFF
// sobre las rutas de escritura, montando la cadena real SessionAuth + RequireAdmin.
//
// Validates: Requirements 5.3, 5.4
//
// Propiedad: *para toda* petición de escritura autenticada con un JWT de sesión
// que embebe un rol arbitrario, la cadena SessionAuth + RequireAdmin la ejecuta
// (200) SI Y SOLO SI el rol es exactamente `admin` (users.RoleAdmin). Cualquier
// otro rol —`viewer`, cadena vacía o cualquier valor aleatorio— se rechaza con
// 403 y el envoltorio de error normalizado (ok=false, errorCode=AUTH_FORBIDDEN),
// sin ejecutar el handler protegido.
//
// Se usa la librería estándar `testing/quick` para generar roles arbitrarios:
// se incluyen deliberadamente `admin` y `viewer` junto con la cadena vacía y
// cadenas aleatorias/basura, de modo que el "si y solo si" se ejercite en ambos
// sentidos (aceptación para `admin`, rechazo para todo lo demás). El token se
// firma siempre con testSecret mediante newToken, embebiendo el rol generado,
// para que SessionAuth lo acepte y deje la identidad en el contexto antes de
// que RequireAdmin la autorice.
//
// Reutiliza, sin modificarlos, los helpers testSecret, newToken (auth_test.go) y
// decodeError (auth_test.go). Todos los identificadores nuevos llevan el prefijo
// rbacPBT* para evitar colisiones con role_test.go y auth_test.go del mismo paquete.
package middleware

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/users"
)

// rbacPBTIterations es el número de iteraciones de la propiedad. Cumple la
// convención de PBT del proyecto (mínimo 100).
const rbacPBTIterations = 300

// rbacPBTWriteMethods enumera los métodos HTTP de escritura sobre los que se
// aplica el control por rol (crear/editar/eliminar y, por extensión, cancelar
// que se expresa como POST). La propiedad debe mantenerse para todos ellos.
var rbacPBTWriteMethods = []string{
	http.MethodPost,
	http.MethodPut,
	http.MethodDelete,
}

// rbacPBTInput es la entrada generada para una iteración de la propiedad: el
// rol embebido en el JWT de sesión y el método HTTP de la ruta de escritura.
type rbacPBTInput struct {
	role   string
	method string
}

// Generate satisface quick.Generator produciendo roles variados (incluyendo los
// casos frontera `admin` y `viewer`, la cadena vacía y basura aleatoria) y un
// método de escritura cualquiera, cubriendo ambos lados del "si y solo si".
func (rbacPBTInput) Generate(rnd *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(rbacPBTInput{
		role:   rbacPBTGenRole(rnd),
		method: rbacPBTWriteMethods[rnd.Intn(len(rbacPBTWriteMethods))],
	})
}

// rbacPBTGenRole genera un rol arbitrario. Garantiza una cobertura adecuada del
// caso aceptado (`admin`) y de los rechazados (`viewer`, vacío, variantes de
// mayúsculas de admin y cadenas aleatorias), para que la propiedad se ejercite
// en ambos sentidos.
func rbacPBTGenRole(rnd *rand.Rand) string {
	switch rnd.Intn(8) {
	case 0, 1:
		// ~1 de cada 4: el único rol que debe aceptarse.
		return users.RoleAdmin
	case 2:
		return users.RoleViewer
	case 3:
		return "" // rol vacío: debe rechazarse
	case 4:
		// Variante con mayúsculas: sensible a mayúsculas, no coincide con "admin".
		return []string{"Admin", "ADMIN", "aDmin", " admin", "admin "}[rnd.Intn(5)]
	case 5:
		// Roles cercanos pero inválidos.
		return []string{"administrator", "superadmin", "root", "editor", "owner"}[rnd.Intn(5)]
	default:
		return rbacPBTRandString(rnd, 1+rnd.Intn(12))
	}
}

// rbacPBTRandString genera una cadena aleatoria para explorar el espacio de
// roles no permitidos. El alfabeto incluye letras, dígitos y algunos símbolos.
func rbacPBTRandString(rnd *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. "
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rnd.Intn(len(alphabet))]
	}
	return string(buf)
}

// rbacPBTBuildRouter arma un router que registra la ruta de escritura dada con
// la cadena real SessionAuth(testSecret) + RequireAdmin, replicando el montaje
// del router de producción (tarea 9.1). El handler final solo responde 200 si
// la autorización lo deja pasar.
func rbacPBTBuildRouter(method, path string) *gin.Engine {
	r := gin.New()
	r.Handle(method, path, SessionAuth(testSecret), RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestRequireAdminRoleAccessIFF verifica la Property 14: a través de
// SessionAuth + RequireAdmin, una petición de escritura con token válido se
// permite (200) si y solo si el rol es exactamente `admin`; cualquier otro rol
// produce 403 con el envoltorio de error normalizado (AUTH_FORBIDDEN).
func TestRequireAdminRoleAccessIFF(t *testing.T) {
	const path = "/write"

	property := func(in rbacPBTInput) bool {
		r := rbacPBTBuildRouter(in.method, path)
		token := newToken(t, "id", "user", in.role, 60)

		req := httptest.NewRequest(in.method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// Oráculo independiente del "si y solo si": solo "admin" autoriza.
		shouldAllow := in.role == users.RoleAdmin

		if shouldAllow {
			return w.Code == http.StatusOK
		}

		// Debe rechazarse con 403 y el envoltorio de error normalizado.
		if w.Code != http.StatusForbidden {
			return false
		}
		body := decodeError(t, w)
		return !body.OK && body.ErrorCode == "AUTH_FORBIDDEN"
	}

	cfg := &quick.Config{MaxCount: rbacPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 14 violada: la escritura se permite si y solo si el rol es admin; cualquier otro rol debe dar 403 AUTH_FORBIDDEN: %v", err)
	}
}

// TestRequireAdminRoleAccessBoundaryRoles complementa la generación aleatoria con
// roles frontera deterministas, confirmando el comportamiento en ambos sentidos
// del "si y solo si" para cada método de escritura.
func TestRequireAdminRoleAccessBoundaryRoles(t *testing.T) {
	const path = "/write"

	cases := []struct {
		name      string
		role      string
		wantAllow bool
	}{
		{"admin permitido", users.RoleAdmin, true},
		{"viewer rechazado", users.RoleViewer, false},
		{"rol vacío rechazado", "", false},
		{"admin con mayúsculas rechazado", "ADMIN", false},
		{"admin con espacio rechazado", " admin", false},
		{"rol arbitrario rechazado", "editor", false},
	}

	for _, method := range rbacPBTWriteMethods {
		for _, tc := range cases {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				r := rbacPBTBuildRouter(method, path)
				token := newToken(t, "id", "user", tc.role, 60)

				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				if tc.wantAllow {
					if w.Code != http.StatusOK {
						t.Fatalf("se esperaba 200 para rol %q, se obtuvo %d (body=%q)", tc.role, w.Code, w.Body.String())
					}
					return
				}

				if w.Code != http.StatusForbidden {
					t.Fatalf("se esperaba 403 para rol %q, se obtuvo %d (body=%q)", tc.role, w.Code, w.Body.String())
				}
				body := decodeError(t, w)
				if body.OK {
					t.Error("el envoltorio de error debe tener ok=false")
				}
				if body.ErrorCode != "AUTH_FORBIDDEN" {
					t.Errorf("errorCode = %q, se esperaba AUTH_FORBIDDEN", body.ErrorCode)
				}
			})
		}
	}
}
