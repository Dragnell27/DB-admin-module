package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/api/middleware"
	"admin-backend/internal/audit"
	"admin-backend/internal/auth"
)

// Claves de contexto usadas por la auditoría para que los handlers puedan
// enriquecer la entrada (p. ej. fijar el id del objeto creado o el errorCode).
const (
	contextKeyAuditObjectID = "audit.objectID"
	contextKeyAuditError    = "audit.errorCode"
	contextKeyAuditMessage  = "audit.message"
	contextKeyAuditBodyID   = "audit.bodyID"
)

// sensitiveKeys son las claves del payload que NUNCA se almacenan en la bitácora
// (contraseñas, tokens de pago, hashes, secretos). Se comparan en minúsculas.
var sensitiveKeys = map[string]struct{}{
	"password":             {},
	"generatedpassword":    {},
	"password_hash":        {},
	"passwordhash":         {},
	"payment_method_token": {},
	"paymentmethodtoken":   {},
	"token":                {},
	"secret":               {},
	"clientsecret":         {},
	// Credenciales de orígenes del constructor de módulos (module-builder):
	// nunca deben persistirse en la bitácora (Req. 12.2, 12.3).
	"apikey":            {},
	"api_key":           {},
	"bearertoken":       {},
	"bearer_token":      {},
	"client_secret":     {},
	"connectionstring":  {},
	"connection_string": {},
	"dsn":               {},
	"tokenurl":          {},
	"token_url":         {},
	// Claves CONTENEDORAS de secretos anidados en los cuerpos de crear/editar de
	// module-builder y connection-templates. Como redact() descarta la entrada
	// completa cuando la clave de primer nivel es sensible, añadirlas aquí
	// elimina el objeto anidado entero (secrets.variableValues, secrets.dsn
	// legado, y los valores FIJOS de la plantilla) de la bitácora (Req. 4.4).
	// Esto corrige además la fuga preexistente de secrets.dsn de los módulos.
	"secrets":     {},
	"fixedvalues": {},
}

// SetAuditObjectID permite a un handler fijar el id del objeto afectado (p. ej.
// el id generado al crear un recurso), que el middleware de auditoría preferirá
// sobre el id inferido de la petición.
func SetAuditObjectID(c *gin.Context, id string) {
	if strings.TrimSpace(id) != "" {
		c.Set(contextKeyAuditObjectID, id)
	}
}

// setAuditError registra en el contexto el errorCode de una respuesta de error,
// para que el middleware lo persista. Lo invoca writeError.
func setAuditError(c *gin.Context, errorCode string) {
	if strings.TrimSpace(errorCode) != "" {
		c.Set(contextKeyAuditError, errorCode)
	}
}

// setAuditMessage registra en el contexto el mensaje legible de una respuesta
// de error, para que el middleware lo persista en la bitácora (p. ej. el motivo
// que devolvió el origen de un módulo). Lo invoca writeError.
func setAuditMessage(c *gin.Context, message string) {
	if strings.TrimSpace(message) != "" {
		c.Set(contextKeyAuditMessage, message)
	}
}

// AuditMiddleware construye el middleware que registra en la bitácora cada
// operación de escritura de `resource`/`action`. Captura el actor, el país y el
// payload (redaccionado) ANTES de ejecutar el handler, deja correr la cadena y,
// en una función diferida, registra el resultado (success/error según el status
// final, incluido un 403 de RequirePermission) junto con el id del objeto
// afectado. El registro es best-effort: un fallo al insertar solo se loguea y
// jamás altera la respuesta.
//
// Debe montarse ANTES de RequirePermission en la cadena de la ruta, para que un
// intento denegado por permiso (que aborta la cadena) también quede auditado.
func AuditMiddleware(repo audit.Repository, resource auth.Resource, action auth.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		if repo == nil {
			c.Next()
			return
		}

		// Capturar el payload de entrada (cuerpo JSON) rebobinando el body para
		// que el handler pueda leerlo después. Al capturarlo (antes de que el
		// handler consuma el body) también se infiere un id de respaldo del
		// cuerpo, porque en el `defer` el body ya puede estar agotado.
		payload, bodyID := capturePayload(c)
		if bodyID != "" {
			c.Set(contextKeyAuditBodyID, bodyID)
		}

		// Registrar al terminar la petición (tras handler o abort por permiso).
		defer func() {
			recordAudit(c, repo, resource, action, payload)
		}()

		c.Next()
	}
}

// AuditModuleMiddleware es la variante de AuditMiddleware para rutas dinámicas
// de módulos (module-builder), donde el recurso es `mod:<slug>` (derivado del
// parámetro de ruta en cada petición) en lugar de un recurso estático fijo. La
// acción es la acción de permiso de la ruta. Comparte la lógica de captura de
// payload redaccionado y registro diferido con AuditMiddleware. Registra SIEMPRE
// (éxito y error): se usa en las acciones de escritura de módulos.
func AuditModuleMiddleware(repo audit.Repository, action auth.Action) gin.HandlerFunc {
	return auditModule(repo, action, false)
}

// AuditModuleMiddlewareOnError es como AuditModuleMiddleware pero SOLO registra
// cuando la operación termina en error (status HTTP >= 400). Se usa en las
// LECTURAS de módulos (p. ej. GET /modules/:slug/list): auditar cada listado
// exitoso llenaría la bitácora de ruido, pero un fallo (origen caído, 403, error
// de OTT) sí debe quedar registrado para poder diagnosticarlo.
func AuditModuleMiddlewareOnError(repo audit.Repository, action auth.Action) gin.HandlerFunc {
	return auditModule(repo, action, true)
}

// auditModule es la implementación común de las dos variantes de auditoría de
// módulos. Si onlyOnError es true, el registro diferido se omite cuando la
// respuesta fue exitosa (2xx).
func auditModule(repo audit.Repository, action auth.Action, onlyOnError bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if repo == nil {
			c.Next()
			return
		}
		resource := auth.DynamicResource(c.Param("slug"))
		payload, bodyID := capturePayload(c)
		if bodyID != "" {
			c.Set(contextKeyAuditBodyID, bodyID)
		}
		defer func() {
			if onlyOnError {
				status := c.Writer.Status()
				if status >= 200 && status < 300 {
					return // lectura exitosa: no se audita (evita ruido)
				}
			}
			recordAudit(c, repo, resource, action, payload)
		}()
		c.Next()
	}
}

// capturePayload lee el cuerpo JSON de la petición (si lo hay), lo rebobina para
// no consumirlo, y devuelve un mapa redaccionado (sin claves sensibles) junto a
// un id inferido del cuerpo (si alguna clave común de id está presente).
// Devuelve (nil, "") si no hay cuerpo JSON interpretable.
func capturePayload(c *gin.Context) (map[string]interface{}, string) {
	if c.Request == nil || c.Request.Body == nil {
		return nil, ""
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, ""
	}
	// Rebobinar siempre, haya o no JSON válido, para no romper al handler.
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 {
		return nil, ""
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, ""
	}
	return redact(parsed), idFromBody(parsed)
}

// idFromBody busca un identificador en el cuerpo JSON, probando claves comunes.
func idFromBody(m map[string]interface{}) string {
	for _, k := range []string{"id", "plan_id", "subscription_id", "customer_id", "external_id"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// redact devuelve una copia superficial del mapa sin las claves sensibles.
func redact(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if _, sensitive := sensitiveKeys[strings.ToLower(k)]; sensitive {
			continue
		}
		out[k] = v
	}
	return out
}

// recordAudit construye y persiste la entrada de auditoría tras la ejecución.
func recordAudit(c *gin.Context, repo audit.Repository, resource auth.Resource, action auth.Action, payload map[string]interface{}) {
	status := c.Writer.Status()
	outcome := audit.OutcomeSuccess
	if status < 200 || status >= 300 {
		outcome = audit.OutcomeError
	}

	actorID, _ := middleware.CurrentUserID(c)
	actorName, _ := middleware.CurrentUsername(c)
	country, _ := CountryFromContext(c)

	entry := audit.Entry{
		ActorID:    actorID,
		ActorName:  actorName,
		Resource:   string(resource),
		Action:     string(action),
		ObjectID:   resolveObjectID(c, resource),
		Country:    country,
		Outcome:    outcome,
		StatusCode: status,
		ErrorCode:  auditErrorCode(c),
		Message:    auditMessage(c),
		Payload:    payload,
	}

	// Best-effort: un fallo al escribir la bitácora no debe afectar la respuesta.
	ctx := context.Background()
	if c.Request != nil && c.Request.Context() != nil {
		ctx = c.Request.Context()
	}
	if err := repo.Insert(ctx, entry); err != nil {
		log.Printf("audit: no se pudo registrar la operación %s/%s: %v", resource, action, err)
	}
}

// auditErrorCode devuelve el errorCode que writeError dejó en el contexto, si lo
// hubo (solo relevante cuando outcome=error).
func auditErrorCode(c *gin.Context) string {
	if v, ok := c.Get(contextKeyAuditError); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// auditMessage devuelve el mensaje legible que writeError dejó en el contexto,
// si lo hubo (p. ej. el motivo del origen de un módulo). Se persiste en la
// bitácora para poder diagnosticar fallos sin revisar logs.
func auditMessage(c *gin.Context) string {
	if v, ok := c.Get(contextKeyAuditMessage); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// resolveObjectID determina el id del objeto afectado. Prefiere el id que el
// handler haya fijado con SetAuditObjectID (p. ej. el id generado al crear); si
// no, lo infiere de los parámetros/cuerpo de la petición según el recurso.
func resolveObjectID(c *gin.Context, resource auth.Resource) string {
	if v, ok := c.Get(contextKeyAuditObjectID); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}

	// Recursos dinámicos de módulo (mod:<slug>): a falta de un id explícito, el
	// slug del módulo (parámetro de ruta) es el mejor identificador para la
	// entrada —útil al diagnosticar fallos de listado de un módulo concreto—.
	if auth.IsDynamicResource(resource) {
		if slug := strings.TrimSpace(c.Param("slug")); slug != "" {
			return slug
		}
	}

	// Candidatos de query por recurso.
	queryKeys := map[auth.Resource][]string{
		auth.ResourceCustomers:     {"customer_id", "email", "external_id"},
		auth.ResourcePlans:         {"plan_id"},
		auth.ResourceSubscriptions: {"subscription_id"},
		auth.ResourceCards:         {"card_id", "customer_id", "email"},
	}
	for _, k := range queryKeys[resource] {
		if v := strings.TrimSpace(c.Query(k)); v != "" {
			return v
		}
	}

	// Id de respaldo inferido del cuerpo al capturarlo (antes de que el handler
	// lo consumiera), guardado en el contexto por el middleware.
	if v, ok := c.Get(contextKeyAuditBodyID); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
