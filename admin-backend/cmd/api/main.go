// Command api es el punto de entrada del servidor HTTP del admin-backend (BFF).
//
// Implementa el ensamblaje de la tarea 9.1: carga la configuración desde el
// entorno, abre la conexión a PostgreSQL, aplica las migraciones del esquema al
// arrancar (design.md "Capa de datos"), construye el cliente OTT y los handlers
// de recurso, arma el router Gin (middlewares, CORS, healthcheck y rutas) y
// escucha en el puerto configurado. main es el único punto de composición: el
// router y los handlers no construyen sus dependencias.
package main

import (
	"log"
	"net/http"

	"admin-backend/internal/api"
	"admin-backend/internal/api/middleware"
	"admin-backend/internal/audit"
	"admin-backend/internal/auth"
	"admin-backend/internal/config"
	"admin-backend/internal/connections"
	"admin-backend/internal/db"
	"admin-backend/internal/engine"
	"admin-backend/internal/modules"
	"admin-backend/internal/ott"
	"admin-backend/internal/ottcache"
	"admin-backend/internal/roles"
	"admin-backend/internal/secrets"
	"admin-backend/internal/users"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("api: error cargando la configuración: %v", err)
	}

	gormDB, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("api: error abriendo la conexión a la base de datos: %v", err)
	}
	defer func() {
		if cerr := db.Close(gormDB); cerr != nil {
			log.Printf("api: advertencia al cerrar la conexión: %v", cerr)
		}
	}()

	// Aplicar migraciones al arrancar (idempotente): deja el esquema `users`
	// disponible antes de atender peticiones (tarea 2.2).
	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("api: error aplicando migraciones: %v", err)
	}

	// Dependencias de dominio e integración OTT.
	usersRepo := users.NewRepository(gormDB)
	rolesRepo := roles.NewRepository(gormDB)
	auditRepo := audit.NewRepository(gormDB)
	ottClient := ott.NewClient(cfg)

	// Caché en memoria de las lecturas OTT (customers/plans/subscriptions),
	// compartido por los tres handlers. TTL configurable (OTT_CACHE_TTL_MINUTES).
	ottReadCache := ottcache.New(cfg.OTTCacheTTL())

	configuredCountries := cfg.ConfiguredCountries()

	// Constructor de módulos (module-builder). Solo se activa si hay clave de
	// cifrado configurada (MODULE_SECRETS_KEY); sin ella, la feature queda
	// desactivada (sus rutas no se registran) y el resto del BFF funciona igual.
	var modulesHandler *api.ModulesHandler
	var connectionsHandler *api.ConnectionsHandler
	var moduleLoader middleware.ModuleActionLoader
	var modulesRepo modules.Repository
	if len(cfg.ModuleSecretsKey) > 0 {
		sealer, serr := secrets.New(cfg.ModuleSecretsKey)
		if serr != nil {
			log.Fatalf("api: MODULE_SECRETS_KEY inválida: %v", serr)
		}
		connRepo := connections.NewRepository(gormDB, sealer)
		modulesRepo = modules.NewRepository(gormDB, sealer, connRepo)
		pgExec := engine.NewPGExecutor(modulesRepo.(engine.SecretProvider), nil)
		rbacReg := modules.NewRBACRegistrar(gormDB)
		// pgExec satisface tanto engine.Executor como engine.PoolInvalidator.
		modulesHandler = api.NewModulesHandler(modulesRepo, pgExec, pgExec, rbacReg)
		connectionsHandler = api.NewConnectionsHandler(connRepo, pgExec)

		// Loader para RequireModulePermission: resuelve las acciones de permiso
		// declaradas por el módulo del :slug (ActionVer para listar/leer;
		// además las acciones de permiso específicas de cada acción).
		moduleLoader = func(c *gin.Context, slug string) ([]auth.Action, bool) {
			def, derr := modulesRepo.FindBySlug(c.Request.Context(), slug)
			if derr != nil {
				return nil, false
			}
			actions := []auth.Action{auth.ActionVer}
			seen := map[auth.Action]struct{}{auth.ActionVer: {}}
			for _, a := range def.Actions {
				pa := auth.Action(a.PermAction)
				if pa == "" {
					continue
				}
				if _, ok := seen[pa]; !ok {
					seen[pa] = struct{}{}
					actions = append(actions, pa)
				}
			}
			return actions, true
		}
		log.Printf("api: constructor de módulos (module-builder) activado")
	} else {
		log.Printf("api: MODULE_SECRETS_KEY ausente; constructor de módulos desactivado")
	}

	deps := api.RouterDeps{
		SessionSecret:       cfg.AdminJWTSecret,
		CORSAllowedOrigin:   cfg.CORSAllowedOrigin,
		DefaultCountry:      cfg.OTTActiveCountry,
		ConfiguredCountries: configuredCountries,
		Auth:                api.NewAuthHandler(usersRepo, rolesRepo, cfg.AdminJWTSecret, cfg.AdminJWTExpiresMinutes),
		Meta:                api.NewMetaHandler(configuredCountries, cfg.OTTActiveCountry),
		Customers:           api.NewCustomersHandler(ottClient, cfg.OTTActiveCountry, ottReadCache),
		Plans:               api.NewPlansHandler(ottClient, cfg.OTTActiveCountry, ottReadCache),
		Subscriptions:       api.NewSubscriptionsHandler(ottClient, cfg.OTTActiveCountry, ottReadCache),
		Cards:               api.NewCardsHandler(ottClient, cfg.OTTActiveCountry),
		Users:               api.NewUsersHandler(usersRepo, rolesRepo, cfg.BcryptCost),
		Roles:               api.NewRolesHandler(rolesRepo, modulesRepo),
		AuditLogs:           api.NewAuditHandler(auditRepo),
		Modules:             modulesHandler,
		Connections:         connectionsHandler,
		ModuleActionLoader:  moduleLoader,
		Audit:               auditRepo,
	}

	log.Printf("api: países OTT configurados: %v (activo por defecto: %s)", configuredCountries, cfg.OTTActiveCountry)

	router := api.NewRouter(deps)

	addr := ":" + cfg.Port
	log.Printf("api: admin-backend escuchando en %s (país activo: %s)", addr, cfg.OTTActiveCountry)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("api: el servidor se detuvo: %v", err)
	}
}
