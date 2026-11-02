// Package api — cableado de rutas del admin-backend (BFF).
//
// Este archivo ensambla el router Gin con los middlewares transversales
// (recovery, logging, CORS), expone el healthcheck público, el login público y
// el grupo protegido por sesión (GET /auth/me y los recursos).
//
// Autorización por permiso (tarea 12.1): cada ruta de recurso pasa por
// middleware.RequirePermission(recurso, acción) según el "Mapa ruta →
// (recurso, acción)" del design, sustituyendo al antiguo middleware de rol
// RequireAdmin. Las lecturas (GET de recurso) exigen el permiso `ver` del
// recurso en el país efectivo; las escrituras (POST/PUT/DELETE, cambiar y
// cancelar) exigen su acción correspondiente (Req. 8.1, 8.5, 8.7, 15.1, 15.2,
// 15.3). GET /auth/me queda bajo sesión (no requiere permiso).
//
// El router no construye las dependencias (handlers, secreto, origen CORS): las
// recibe ya ensambladas en RouterDeps, de modo que cmd/api/main.go sea el único
// punto de composición y las pruebas de integración (tarea 9.2) puedan inyectar
// handlers con un OTT simulado.
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/api/middleware"
	"admin-backend/internal/audit"
	"admin-backend/internal/auth"
)

// RouterDeps agrupa las dependencias que el router necesita para registrar las
// rutas. Se inyectan ya construidas desde cmd/api/main.go (o desde una prueba
// de integración), manteniendo este paquete libre de lógica de arranque.
type RouterDeps struct {
	// SessionSecret es el ADMIN_JWT_SECRET con el que el middleware de sesión
	// valida los JWT de sesión del admin.
	SessionSecret string
	// CORSAllowedOrigin es el único origen (el del admin-frontend) para el que
	// el BFF habilita CORS (design.md "Security" → CORS).
	CORSAllowedOrigin string

	// DefaultCountry es el país OTT por defecto (OTT_ACTIVE_COUNTRY) usado
	// cuando una petición no envía la cabecera X-Country.
	DefaultCountry string
	// ConfiguredCountries es el conjunto de países con credenciales OTT; el
	// middleware de país valida contra él los valores de X-Country y el selector
	// del front se puebla con esta lista.
	ConfiguredCountries []string

	Auth          *AuthHandler
	Meta          *MetaHandler
	Customers     *CustomersHandler
	Plans         *PlansHandler
	Subscriptions *SubscriptionsHandler
	Cards         *CardsHandler
	Users         *UsersHandler
	Roles         *RolesHandler
	AuditLogs     *AuditHandler

	// Modules es el handler del constructor de módulos (module-builder): CRUD de
	// definiciones y ejecución dinámica. Puede ser nil (la feature queda
	// desactivada: no se registran sus rutas).
	Modules *ModulesHandler
	// Connections es el handler de plantillas de conexión (connection-templates):
	// CRUD de plantillas. Puede ser nil (la feature queda desactivada: no se
	// registran sus rutas), igual que Modules, condicionado a MODULE_SECRETS_KEY.
	Connections *ConnectionsHandler
	// ModuleActionLoader resuelve las acciones de permiso declaradas por un
	// módulo (para RequireModulePermission). Puede ser nil.
	ModuleActionLoader middleware.ModuleActionLoader

	// Audit es el repositorio de la bitácora; el middleware de auditoría lo usa
	// para registrar las operaciones de escritura. Puede ser nil (p. ej. en
	// pruebas que no auditan), en cuyo caso el middleware es un no-op.
	Audit audit.Repository
}

// NewRouter construye y configura el *gin.Engine del admin-backend con todos
// sus middlewares y rutas, listo para servir.
//
// Orden de middlewares globales: Recovery (captura panics y responde 500 sin
// tumbar el proceso) → Logger (traza de acceso) → CORS (cabeceras del origen
// permitido y respuesta a preflight). Después se registran: GET /health
// (público), POST /auth/login (público) y el grupo autenticado con GET /auth/me
// y los recursos; dentro del grupo, cada ruta de recurso añade
// RequirePermission(recurso, acción).
func NewRouter(deps RouterDeps) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gin.Logger())
	engine.Use(corsMiddleware(deps.CORSAllowedOrigin))

	// Healthcheck público (Req. 21.4): sin autenticación, para que el
	// orquestador (Docker Compose) verifique la salud del servicio.
	engine.GET("/health", healthcheck)

	// Autenticación: login público; el resto del árbol de auth y recursos vive
	// bajo el middleware de sesión.
	if deps.Auth != nil {
		engine.POST("/auth/login", deps.Auth.Login)
	}

	// Grupo protegido: toda ruta aquí exige un JWT de sesión válido (401 si
	// falta o es inválido, lo que dispara el logout en el front, Req. 2.5).
	// Además, el middleware de país resuelve el país efectivo de cada petición
	// (cabecera X-Country o el país por defecto) y lo deja en el contexto para
	// los handlers de recurso; un X-Country no soportado produce 400.
	authenticated := engine.Group("/")
	authenticated.Use(middleware.SessionAuth(deps.SessionSecret))
	authenticated.Use(CountryResolver(deps.DefaultCountry, deps.ConfiguredCountries))

	registerAuthRoutes(authenticated, deps.Auth)
	registerMetaRoutes(authenticated, deps.Meta)
	registerCustomerRoutes(authenticated, deps.Customers, deps.Audit)
	registerPlanRoutes(authenticated, deps.Plans, deps.Audit)
	registerSubscriptionRoutes(authenticated, deps.Subscriptions, deps.Audit)
	registerCardRoutes(authenticated, deps.Cards, deps.Audit)
	registerUserRoutes(authenticated, deps.Users, deps.Audit)
	registerRoleRoutes(authenticated, deps.Roles, deps.Audit)
	registerAuditRoutes(authenticated, deps.AuditLogs)
	registerModuleRoutes(authenticated, deps.Modules, deps.ModuleActionLoader, deps.Audit)
	registerConnectionRoutes(authenticated, deps.Connections, deps.Audit)

	return engine
}

// registerConnectionRoutes registra el CRUD de plantillas de conexión
// (connection-templates). El recurso estático `connections` se transporta en
// `modperms` bajo el slug reservado auth.ConnectionsSlug, por lo que su
// autorización usa RequireModuleDefsPermission con ese slug (mismo guard que
// module-builder). Las escrituras se auditan ANTES de autorizar.
func registerConnectionRoutes(rg *gin.RouterGroup, h *ConnectionsHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	conn := auth.ConnectionsSlug

	rg.GET("/connection-templates",
		middleware.RequireModuleDefsPermission(conn, auth.ActionVer), h.List)
	rg.GET("/connection-templates/:id",
		middleware.RequireModuleDefsPermission(conn, auth.ActionVer), h.Get)
	rg.POST("/connection-templates",
		AuditMiddleware(aud, auth.ResourceConnections, auth.ActionCrear),
		middleware.RequireModuleDefsPermission(conn, auth.ActionCrear), h.Create)
	rg.PUT("/connection-templates/:id",
		AuditMiddleware(aud, auth.ResourceConnections, auth.ActionEditar),
		middleware.RequireModuleDefsPermission(conn, auth.ActionEditar), h.Update)
	rg.DELETE("/connection-templates/:id",
		AuditMiddleware(aud, auth.ResourceConnections, auth.ActionEliminar),
		middleware.RequireModuleDefsPermission(conn, auth.ActionEliminar), h.Delete)
}

// registerModuleRoutes registra las rutas del constructor de módulos
// (module-builder): el CRUD de definiciones (recurso estático module-builder) y
// las rutas dinámicas de ejecución (recurso dinámico mod:<slug>). Las escrituras
// se auditan ANTES de la autorización, igual que el resto de recursos.
//
// El recurso estático module-builder se transporta en el claim `modperms` bajo
// el slug reservado auth.ModuleDefsSlug, por lo que su autorización usa
// RequireModulePermission con un loader fijo que declara las 4 acciones CRUD.
func registerModuleRoutes(rg *gin.RouterGroup, h *ModulesHandler, loader middleware.ModuleActionLoader, aud audit.Repository) {
	if h == nil {
		return
	}

	// El CRUD de definiciones autoriza contra el recurso estático module-builder,
	// transportado en modperms bajo el slug reservado auth.ModuleDefsSlug. No
	// depende del :slug del path (ese es el id del módulo concreto).
	defs := auth.ModuleDefsSlug

	rg.GET("/module-defs",
		middleware.RequireModuleDefsPermission(defs, auth.ActionVer), h.ListDefs)
	rg.GET("/module-defs/:slug",
		middleware.RequireModuleDefsPermission(defs, auth.ActionVer), h.GetDef)
	rg.POST("/module-defs",
		AuditMiddleware(aud, auth.ResourceModuleBuilder, auth.ActionCrear),
		middleware.RequireModuleDefsPermission(defs, auth.ActionCrear), h.CreateDef)
	rg.PUT("/module-defs/:slug",
		AuditMiddleware(aud, auth.ResourceModuleBuilder, auth.ActionEditar),
		middleware.RequireModuleDefsPermission(defs, auth.ActionEditar), h.UpdateDef)
	rg.DELETE("/module-defs/:slug",
		AuditMiddleware(aud, auth.ResourceModuleBuilder, auth.ActionEliminar),
		middleware.RequireModuleDefsPermission(defs, auth.ActionEliminar), h.DeleteDef)

	// Descubrimiento de módulos visibles (solo sesión; filtra por ver dentro).
	rg.GET("/modules", h.ListVisible)

	// Ejecución dinámica: listar (ver) y ejecutar acciones (acción del módulo).
	// El listado audita SOLO los fallos (origen caído, 403, error de OTT) para
	// poder diagnosticarlos sin llenar la bitácora con cada lectura exitosa.
	rg.GET("/modules/:slug/list",
		AuditModuleMiddlewareOnError(aud, auth.ActionVer),
		middleware.RequireModulePermission(loader, auth.ActionVer), h.List)
	// Opciones (id/etiqueta) de un campo con origen dinámico: es una lectura,
	// exige el permiso `ver` del módulo y audita solo los fallos.
	rg.GET("/modules/:slug/options/:field",
		AuditModuleMiddlewareOnError(aud, auth.ActionVer),
		middleware.RequireModulePermission(loader, auth.ActionVer), h.FieldOptions)
	rg.POST("/modules/:slug/actions/:actionId",
		AuditModuleMiddleware(aud, auth.ActionEditar),
		middleware.RequireModulePermission(loader, auth.ActionVer), h.ExecAction)
	// Ruta dedicada de LECTURA para acciones SELECT: ejecuta el listado con el
	// WHERE derivado de los campos llenos de la acción. Mismos middlewares que
	// GET /list (permiso `ver`, auditoría solo en error): una acción SELECT es
	// una lectura, nunca una escritura.
	rg.POST("/modules/:slug/actions/:actionId/list",
		AuditModuleMiddlewareOnError(aud, auth.ActionVer),
		middleware.RequireModulePermission(loader, auth.ActionVer), h.ExecActionList)
}

// healthcheck responde el estado del servicio con el envoltorio uniforme del
// BFF. No consulta dependencias externas: confirma que el proceso HTTP atiende.
func healthcheck(c *gin.Context) {
	writeOK(c, http.StatusOK, gin.H{"status": "ok"}, "")
}

// registerAuthRoutes registra GET /auth/me bajo sesión (devuelve la identidad
// del administrador autenticado desde los claims del token, Req. 4, 5).
func registerAuthRoutes(rg *gin.RouterGroup, h *AuthHandler) {
	if h == nil {
		return
	}
	rg.GET("/auth/me", h.Me)
	// Reemisión del token con los permisos actuales del usuario (botón "Recargar
	// permisos"): solo requiere sesión (identifica al usuario por el claim), no
	// un permiso de recurso, y no se audita.
	rg.POST("/auth/refresh-permissions", h.RefreshPermissions)
}

// registerMetaRoutes registra GET /meta/countries: devuelve los países
// configurados (con credenciales OTT) y el país activo por defecto, para que el
// front pueble el selector de país. Solo requiere sesión (no es un recurso
// administrable).
func registerMetaRoutes(rg *gin.RouterGroup, h *MetaHandler) {
	if h == nil {
		return
	}
	// /meta/countries queda accesible para cualquier usuario autenticado: solo
	// devuelve la lista de países configurados que la UI necesita para el
	// selector de país. No es un recurso de negocio administrable.
	rg.GET("/meta/countries", h.Countries)
}

// registerCustomerRoutes registra las rutas de Customers. GET exige (customers,
// ver); POST exige (customers, crear) y PUT (customers, editar), todo en el país
// efectivo (Req. 8.1, 8.5, 8.7). El contrato OTT no admite DELETE de Customers.
func registerCustomerRoutes(rg *gin.RouterGroup, h *CustomersHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/customers", middleware.RequirePermission(auth.ResourceCustomers, auth.ActionVer), h.List)
	rg.POST("/customers",
		AuditMiddleware(aud, auth.ResourceCustomers, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourceCustomers, auth.ActionCrear), h.Create)
	rg.PUT("/customers",
		AuditMiddleware(aud, auth.ResourceCustomers, auth.ActionEditar),
		middleware.RequirePermission(auth.ResourceCustomers, auth.ActionEditar), h.Edit)
	// Refresco manual del caché (botón "Recargar"): exige (customers, recargar).
	rg.POST("/customers/refresh", middleware.RequirePermission(auth.ResourceCustomers, auth.ActionRecargar), h.Refresh)
}

// registerPlanRoutes registra las rutas de Plans (CRUD completo). GET exige
// (plans, ver); POST (plans, crear); PUT (plans, editar); DELETE (plans,
// eliminar), todo en el país efectivo (Req. 8.1, 8.5, 8.7).
func registerPlanRoutes(rg *gin.RouterGroup, h *PlansHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/plans", middleware.RequirePermission(auth.ResourcePlans, auth.ActionVer), h.List)
	rg.POST("/plans",
		AuditMiddleware(aud, auth.ResourcePlans, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourcePlans, auth.ActionCrear), h.Create)
	rg.PUT("/plans",
		AuditMiddleware(aud, auth.ResourcePlans, auth.ActionEditar),
		middleware.RequirePermission(auth.ResourcePlans, auth.ActionEditar), h.Edit)
	rg.DELETE("/plans",
		AuditMiddleware(aud, auth.ResourcePlans, auth.ActionEliminar),
		middleware.RequirePermission(auth.ResourcePlans, auth.ActionEliminar), h.Delete)
	// Refresco manual del caché (botón "Recargar"): exige (plans, recargar).
	rg.POST("/plans/refresh", middleware.RequirePermission(auth.ResourcePlans, auth.ActionRecargar), h.Refresh)
}

// registerSubscriptionRoutes registra las rutas de Subscriptions. GET exige
// (subscriptions, ver); POST (subscriptions, crear); PUT /change
// (subscriptions, cambiar); POST /cancel (subscriptions, cancelar), todo en el
// país efectivo (Req. 8.1, 8.5, 8.7).
func registerSubscriptionRoutes(rg *gin.RouterGroup, h *SubscriptionsHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/subscriptions", middleware.RequirePermission(auth.ResourceSubscriptions, auth.ActionVer), h.List)
	rg.POST("/subscriptions",
		AuditMiddleware(aud, auth.ResourceSubscriptions, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourceSubscriptions, auth.ActionCrear), h.Create)
	rg.PUT("/subscriptions/change",
		AuditMiddleware(aud, auth.ResourceSubscriptions, auth.ActionCambiar),
		middleware.RequirePermission(auth.ResourceSubscriptions, auth.ActionCambiar), h.Change)
	rg.POST("/subscriptions/cancel",
		AuditMiddleware(aud, auth.ResourceSubscriptions, auth.ActionCancelar),
		middleware.RequirePermission(auth.ResourceSubscriptions, auth.ActionCancelar), h.Cancel)
	// Refresco manual del caché (botón "Recargar"): exige (subscriptions, recargar).
	rg.POST("/subscriptions/refresh", middleware.RequirePermission(auth.ResourceSubscriptions, auth.ActionRecargar), h.Refresh)
}

// registerCardRoutes registra las rutas de Cards. GET exige (cards, ver); POST
// (cards, crear); DELETE (cards, eliminar), todo en el país efectivo (Req. 8.1,
// 8.5, 8.7). El contrato OTT no admite edición de Cards.
func registerCardRoutes(rg *gin.RouterGroup, h *CardsHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/cards", middleware.RequirePermission(auth.ResourceCards, auth.ActionVer), h.ListByCustomer)
	rg.POST("/cards",
		AuditMiddleware(aud, auth.ResourceCards, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourceCards, auth.ActionCrear), h.Create)
	rg.DELETE("/cards",
		AuditMiddleware(aud, auth.ResourceCards, auth.ActionEliminar),
		middleware.RequirePermission(auth.ResourceCards, auth.ActionEliminar), h.Delete)
}

// registerUserRoutes registra las rutas de administración RBAC de usuarios. Las
// lecturas (GET /users y GET /users/:id) exigen (users, ver); las escrituras
// exigen (users, crear) para POST /users y (users, editar) para PUT /users y
// POST /users/reset-password (restablecer contraseña es una edición del
// usuario), todo en el país efectivo (Req. 8.1, 8.5, 8.7, 10.1, 10.3). Tolera
// h == nil como el resto de register* para no romper el wiring en pruebas que
// no inyectan este handler.
func registerUserRoutes(rg *gin.RouterGroup, h *UsersHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/users", middleware.RequirePermission(auth.ResourceUsers, auth.ActionVer), h.List)
	rg.GET("/users/:id", middleware.RequirePermission(auth.ResourceUsers, auth.ActionVer), h.Detail)
	rg.POST("/users",
		AuditMiddleware(aud, auth.ResourceUsers, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourceUsers, auth.ActionCrear), h.Create)
	rg.PUT("/users",
		AuditMiddleware(aud, auth.ResourceUsers, auth.ActionEditar),
		middleware.RequirePermission(auth.ResourceUsers, auth.ActionEditar), h.Edit)
	rg.POST("/users/reset-password",
		AuditMiddleware(aud, auth.ResourceUsers, auth.ActionEditar),
		middleware.RequirePermission(auth.ResourceUsers, auth.ActionEditar), h.ResetPassword)
}

// registerRoleRoutes registra las rutas de administración RBAC de roles. Las
// lecturas (GET /roles y GET /roles/:id) exigen (roles, ver); las escrituras
// exigen (roles, crear) para POST /roles, (roles, editar) para PUT /roles y
// (roles, eliminar) para DELETE /roles/:id, todo en el país efectivo (Req. 8.1,
// 8.5, 8.7, 11.1, 11.3). Tolera h == nil como el resto de register* para no
// romper el wiring en pruebas que no inyectan este handler.
func registerRoleRoutes(rg *gin.RouterGroup, h *RolesHandler, aud audit.Repository) {
	if h == nil {
		return
	}
	rg.GET("/roles", middleware.RequirePermission(auth.ResourceRoles, auth.ActionVer), h.List)
	// Catálogo de módulos para la UI de roles (recursos dinámicos mod:<slug>).
	// Convive de forma segura con GET /roles/:id en gin v1.12.0; es una lectura,
	// no se audita.
	rg.GET("/roles/module-catalog", middleware.RequirePermission(auth.ResourceRoles, auth.ActionVer), h.ModuleCatalog)
	rg.GET("/roles/:id", middleware.RequirePermission(auth.ResourceRoles, auth.ActionVer), h.Detail)
	rg.POST("/roles",
		AuditMiddleware(aud, auth.ResourceRoles, auth.ActionCrear),
		middleware.RequirePermission(auth.ResourceRoles, auth.ActionCrear), h.Create)
	rg.PUT("/roles",
		AuditMiddleware(aud, auth.ResourceRoles, auth.ActionEditar),
		middleware.RequirePermission(auth.ResourceRoles, auth.ActionEditar), h.Edit)
	rg.DELETE("/roles/:id",
		AuditMiddleware(aud, auth.ResourceRoles, auth.ActionEliminar),
		middleware.RequirePermission(auth.ResourceRoles, auth.ActionEliminar), h.Delete)
}

// registerAuditRoutes registra GET /audit-logs (consulta de la bitácora),
// protegido por el permiso (audit, ver). Tolera h == nil.
func registerAuditRoutes(rg *gin.RouterGroup, h *AuditHandler) {
	if h == nil {
		return
	}
	rg.GET("/audit-logs", middleware.RequirePermission(auth.ResourceAudit, auth.ActionVer), h.List)
}

// corsMiddleware habilita CORS únicamente para el origen del admin-frontend
// configurado por entorno (design.md "Security" → CORS). Se implementa a mano
// para no añadir una dependencia nueva: refleja el origen permitido, admite
// credenciales y las cabeceras/métodos que usa el front (Authorization,
// Content-Type y X-Country para el selector de país; GET/POST/PUT/DELETE/OPTIONS),
// y responde al preflight OPTIONS con 204 cortando la cadena.
//
// Si allowedOrigin está vacío, no se emite Access-Control-Allow-Origin (CORS
// deshabilitado); las peticiones del mismo origen siguen funcionando.
func corsMiddleware(allowedOrigin string) gin.HandlerFunc {
	allowedOrigin = strings.TrimSpace(allowedOrigin)

	return func(c *gin.Context) {
		if allowedOrigin != "" {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", allowedOrigin)
			// Al reflejar un origen específico (no "*"), Vary: Origin evita que
			// caches sirvan la respuesta a un origen distinto.
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Country")
			h.Set("Access-Control-Max-Age", "86400")
		}

		// Preflight: responder sin pasar a los handlers.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
