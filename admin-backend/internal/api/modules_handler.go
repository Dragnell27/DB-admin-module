package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/api/middleware"
	"admin-backend/internal/auth"
	"admin-backend/internal/engine"
	"admin-backend/internal/modules"
)

// ModulesHandler expone el CRUD de definiciones de módulo (module-builder) y la
// ejecución dinámica de módulos (listar y acciones). El frontend nunca llama a
// orígenes externos: este handler, a través del motor (engine), es el único
// ejecutor (Req. 13.1, 13.2).
type ModulesHandler struct {
	repo        modules.Repository
	pg          engine.Executor
	invalidator engine.PoolInvalidator
	rbacReg     ModuleRBACRegistrar
}

// ModuleRBACRegistrar registra/elimina el recurso dinámico y las acciones de un
// módulo en el RBAC al crear/editar/eliminar (Req. 11.1, 11.2, 2.5). Se expresa
// como interfaz para inyectar la implementación (o un no-op en pruebas).
type ModuleRBACRegistrar interface {
	RegisterModule(def modules.Definition) error
	UnregisterModule(slug string) error
}

// NewModulesHandler construye el handler con el repositorio de definiciones, el
// ejecutor Postgres (único origen soportado), el invalidador de pools (para
// invalidar tras editar un módulo; puede ser nil = no-op) y el registrador RBAC.
func NewModulesHandler(repo modules.Repository, pgExec engine.Executor, invalidator engine.PoolInvalidator, rbacReg ModuleRBACRegistrar) *ModulesHandler {
	return &ModulesHandler{repo: repo, pg: pgExec, invalidator: invalidator, rbacReg: rbacReg}
}

// executorFor selecciona el ejecutor según el tipo de origen del módulo. El
// único origen soportado es Postgres; cualquier otro tipo se rechaza.
func (h *ModulesHandler) executorFor(def modules.Definition) (engine.Executor, error) {
	switch def.SourceType {
	case modules.SourcePostgres:
		return h.pg, nil
	default:
		return nil, errors.New("tipo de origen no soportado")
	}
}

// --- CRUD de definiciones ---------------------------------------------------

// ListDefs maneja GET /module-defs: lista todas las definiciones (sin secretos).
func (h *ModulesHandler) ListDefs(c *gin.Context) {
	defs, err := h.repo.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar los módulos")
		return
	}
	writeOK(c, http.StatusOK, gin.H{"items": defs}, "")
}

// GetDef maneja GET /module-defs/:slug: devuelve una definición (sin secretos).
func (h *ModulesHandler) GetDef(c *gin.Context) {
	def, err := h.repo.FindBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, modules.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo obtener el módulo")
		return
	}
	writeOK(c, http.StatusOK, def, "")
}

// moduleDefBody es el cuerpo de creación/edición de una definición: la
// definición pública más las credenciales en claro (que se cifran al persistir
// y nunca se devuelven).
type moduleDefBody struct {
	modules.Definition
	Secrets modules.Secrets `json:"secrets"`
}

// CreateDef maneja POST /module-defs: crea una definición y registra su recurso
// RBAC. Slug duplicado -> E409; slug/nombre inválidos -> E422.
func (h *ModulesHandler) CreateDef(c *gin.Context) {
	var body moduleDefBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Cuerpo de la petición inválido")
		return
	}
	def := body.Definition
	if err := h.repo.Create(c.Request.Context(), &def, body.Secrets); err != nil {
		h.writeDefError(c, err)
		return
	}
	SetAuditObjectID(c, def.Slug)
	if h.rbacReg != nil {
		if err := h.rbacReg.RegisterModule(def); err != nil {
			writeError(c, http.StatusInternalServerError, "E500", "El módulo se creó pero no se pudo registrar en el control de acceso")
			return
		}
	}
	writeOK(c, http.StatusCreated, gin.H{"slug": def.Slug, "id": def.ID}, "Módulo creado")
}

// UpdateDef maneja PUT /module-defs/:slug: reemplaza la definición y re-registra
// su recurso RBAC. Secretos vacíos conservan los previos.
func (h *ModulesHandler) UpdateDef(c *gin.Context) {
	var body moduleDefBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Cuerpo de la petición inválido")
		return
	}
	def := body.Definition
	def.Slug = c.Param("slug") // el slug lo fija la ruta (inmutable)
	SetAuditObjectID(c, def.Slug)
	if err := h.repo.Update(c.Request.Context(), &def, body.Secrets); err != nil {
		h.writeDefError(c, err)
		return
	}
	if h.rbacReg != nil {
		if err := h.rbacReg.RegisterModule(def); err != nil {
			writeError(c, http.StatusInternalServerError, "E500", "El módulo se actualizó pero no se pudo registrar en el control de acceso")
			return
		}
	}
	// Invalidar el pool del módulo editado para que la próxima ejecución reabra
	// con el DSN efectivo recalculado (Req. 3.5). No se reutiliza h.pg porque su
	// tipo (engine.Executor) no expone Invalidate.
	if h.invalidator != nil {
		h.invalidator.Invalidate(def.Slug)
	}
	writeOK(c, http.StatusOK, gin.H{"slug": def.Slug}, "Módulo actualizado")
}

// DeleteDef maneja DELETE /module-defs/:slug: elimina la definición y limpia su
// recurso RBAC.
func (h *ModulesHandler) DeleteDef(c *gin.Context) {
	slug := c.Param("slug")
	SetAuditObjectID(c, slug)
	if err := h.repo.Delete(c.Request.Context(), slug); err != nil {
		h.writeDefError(c, err)
		return
	}
	if h.rbacReg != nil {
		if err := h.rbacReg.UnregisterModule(slug); err != nil {
			writeError(c, http.StatusInternalServerError, "E500", "El módulo se eliminó pero no se pudo limpiar el control de acceso")
			return
		}
	}
	writeOK(c, http.StatusOK, gin.H{"slug": slug}, "Módulo eliminado")
}

// writeDefError traduce los errores de negocio del repositorio a respuestas.
func (h *ModulesHandler) writeDefError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, modules.ErrNotFound):
		writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
	case errors.Is(err, modules.ErrSlugTaken):
		writeError(c, http.StatusConflict, "E409", "El identificador del módulo ya está en uso")
	case errors.Is(err, modules.ErrInvalidSlug):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "El identificador del módulo es inválido (se espera kebab-case)")
	case errors.Is(err, modules.ErrEmptyName):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "El nombre del módulo es obligatorio")
	case errors.Is(err, modules.ErrTemplateRequired):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "El módulo debe referenciar una plantilla de conexión existente")
	case errors.Is(err, modules.ErrNoPrimaryKey):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "Marca al menos una columna como llave primaria para las acciones de editar o eliminar")
	case errors.Is(err, modules.ErrMultiTableValidation):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo procesar el módulo")
	}
}

// --- Descubrimiento ---------------------------------------------------------

// ListVisible maneja GET /modules: devuelve los módulos para los que el usuario
// tiene permiso `ver` en el país efectivo, sin credenciales, para el sidebar
// (Req. 11.3).
func (h *ModulesHandler) ListVisible(c *gin.Context) {
	defs, err := h.repo.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar los módulos")
		return
	}
	modperms, _ := middleware.CurrentModulePermissions(c)
	country, _ := CountryFromContext(c)

	visible := make([]moduleSummary, 0, len(defs))
	for _, d := range defs {
		if auth.HasModulePermission(modperms, d.Slug, auth.ActionVer, country) {
			visible = append(visible, moduleSummary{Slug: d.Slug, Name: d.Name, MenuPath: d.MenuPath})
		}
	}
	writeOK(c, http.StatusOK, gin.H{"items": visible}, "")
}

// moduleSummary es la forma mínima de un módulo para el menú lateral. MenuPath
// es la ruta jerárquica libre (organizativa) para construir el árbol de menú.
type moduleSummary struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	MenuPath string `json:"menuPath"`
}

// --- Ejecución dinámica -----------------------------------------------------

// List maneja GET /modules/:slug/list: ejecuta la Acción Listar del módulo.
func (h *ModulesHandler) List(c *gin.Context) {
	def, err := h.repo.FindBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, modules.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo cargar el módulo")
		return
	}
	exec, err := h.executorFor(*def)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "Tipo de origen no soportado")
		return
	}

	country, _ := CountryFromContext(c)
	q := parseListQuery(c)
	rows, page, err := exec.List(c.Request.Context(), *def, country, q)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrWhitelist):
			// Mensaje NEUTRO: ErrWhitelist también puede originarse en checkTable,
			// buildJoinClause o selectColumns* (visualización/PK/condición), no solo
			// en la búsqueda del usuario; el origen concreto queda solo en logs.
			writeError(c, http.StatusUnprocessableEntity, "E422_PG_WHITELIST",
				"La solicitud referencia tablas o columnas no permitidas")
		default:
			writeError(c, http.StatusBadGateway, "E502",
				"El origen del módulo no respondió correctamente: "+err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"items": rows,
			"pagination": gin.H{
				"totalRecords":   page.TotalRecords,
				"page":           page.Page,
				"recordsPerPage": page.RecordsPerPage,
				"totalPages":     page.TotalPages,
			},
		},
		"message": "",
	})
}

// FieldOptions maneja GET /modules/:slug/options/:field: devuelve las opciones
// (id/etiqueta) de un campo con Source=="dynamic", obtenidas en tiempo de
// ejecución de otra tabla de la MISMA conexión del módulo. El query param
// ?action=<actionKey> desambigua cuando varias acciones declaran un campo
// dinámico con el mismo nombre: con ?action se busca SOLO dentro de esa acción;
// sin él, la primera coincidencia dinámica por nombre recorriendo las acciones.
func (h *ModulesHandler) FieldOptions(c *gin.Context) {
	def, err := h.repo.FindBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, modules.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo cargar el módulo")
		return
	}

	// La fuente dinámica a resolver proviene de una COLUMNA (?column=true) o, en
	// la ruta histórica, de un campo de ACCIÓN. Con ?column=true manda la columna
	// (precedencia columna>acción): se resuelve su valueSource.dynamic desde el
	// displayMapping, desambiguando homónimas con &sourceTable=. Sin ?column, se
	// conserva intacta la ruta de acción (findDynamicField + ?action).
	var src modules.DynamicSource
	if c.Query("column") != "" {
		principal := modules.MultiTableFromFetchMapping(def.FetchMapping).PrincipalTable
		resolved, ok := resolveColumnDynamicSource(def, c.Param("field"), c.Query("sourceTable"), principal)
		if !ok {
			writeError(c, http.StatusNotFound, "E404", "La columna no tiene una fuente de valores dinámica")
			return
		}
		src = resolved
	} else {
		field, ok := findDynamicField(def.Actions, c.Param("field"), c.Query("action"))
		if !ok {
			writeError(c, http.StatusNotFound, "E404", "Campo dinámico no encontrado en el módulo")
			return
		}
		// Un campo dinámico clásico lleva el catálogo en DynamicSource; una captura
		// (capture-from-catalog) lo lleva en Strategy.Catalog. findDynamicField
		// garantiza que exactamente uno está presente.
		if field.DynamicSource != nil {
			src = *field.DynamicSource
		} else {
			src = *field.Strategy.Catalog
		}
	}

	// Solo el camino Postgres soporta orígenes dinámicos. Se valida el tipo de
	// origen y se asegura el tipo concreto *PGExecutor (Options vive ahí).
	if _, err := h.executorFor(*def); err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "Tipo de origen no soportado")
		return
	}
	pg, ok := h.pg.(*engine.PGExecutor)
	if !ok {
		writeError(c, http.StatusInternalServerError, "E500", "El ejecutor del módulo no soporta opciones dinámicas")
		return
	}

	opts, err := pg.Options(c.Request.Context(), *def, src)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrWhitelist):
			writeError(c, http.StatusUnprocessableEntity, "E422_PG_WHITELIST", "El origen de las opciones referencia tablas o columnas no permitidas")
		case errors.Is(err, engine.ErrValidation):
			writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "El origen de las opciones está incompleto: "+err.Error())
		default:
			writeError(c, http.StatusBadGateway, "E502", "El origen del módulo no pudo obtener las opciones: "+err.Error())
		}
		return
	}
	writeOK(c, http.StatusOK, gin.H{"items": opts}, "")
}

// findDynamicField localiza un campo dinámico por nombre. Con actionKey no
// vacío, busca SOLO dentro de esa acción (si la acción o el campo no existen,
// devuelve false). Sin actionKey, devuelve la primera coincidencia dinámica por
// nombre recorriendo las acciones. Solo se considera dinámico un campo con
// Source=="dynamic" y DynamicSource != nil.
func findDynamicField(actions []modules.Action, field, actionKey string) (modules.FieldSpec, bool) {
	isDynamic := func(f modules.FieldSpec) bool {
		if f.Name != field {
			return false
		}
		// Campo dinámico clásico: catálogo en DynamicSource.
		if f.Source == modules.FieldSourceDynamic && f.DynamicSource != nil {
			return true
		}
		// Captura de catálogo (fillStrategy): catálogo en Strategy.Catalog. El
		// selector SIEMPRE invoca con ?action=<actionKey>, y el field del path es
		// el name completo (incluido el prefijo __capture_).
		if f.Source == modules.FieldSourceFillStrategy &&
			f.Strategy != nil &&
			f.Strategy.Kind == modules.StrategyCaptureFromCatalog &&
			f.Strategy.Catalog != nil {
			return true
		}
		return false
	}
	if actionKey != "" {
		act, ok := findAction(actions, actionKey)
		if !ok {
			return modules.FieldSpec{}, false
		}
		for _, f := range act.InputFields {
			if isDynamic(f) {
				return f, true
			}
		}
		return modules.FieldSpec{}, false
	}
	for _, act := range actions {
		for _, f := range act.InputFields {
			if isDynamic(f) {
				return f, true
			}
		}
	}
	return modules.FieldSpec{}, false
}

// resolveColumnDynamicSource localiza la fuente dinámica declarada por una
// COLUMNA del módulo (valueSource.kind=="dynamic"). Resuelve la identidad de la
// columna por (field, sourceTable): un sourceTable vacío equivale a la tabla
// principal (compatibilidad N=1), por lo que ante columnas homónimas sin
// sourceTable se toma la de la principal. Devuelve el DynamicSource por valor y
// ok=true solo si la columna tiene ValueSource!=nil, Kind=="dynamic" y
// DynamicSource!=nil; en cualquier otro caso (columna estática, sin fuente o
// inexistente) devuelve ok=false para que el handler responda 404.
func resolveColumnDynamicSource(def *modules.Definition, field, sourceTable, principal string) (modules.DynamicSource, bool) {
	cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, principal)
	resolvedTable := sourceTable
	if resolvedTable == "" {
		resolvedTable = principal
	}
	for _, c := range cols {
		if c.Field != field || c.SourceTable != resolvedTable {
			continue
		}
		if c.ValueSource != nil && c.ValueSource.Kind == "dynamic" && c.ValueSource.DynamicSource != nil {
			return *c.ValueSource.DynamicSource, true
		}
		return modules.DynamicSource{}, false
	}
	return modules.DynamicSource{}, false
}

// ExecAction maneja POST /modules/:slug/actions/:actionId: ejecuta una acción
// del módulo tras validar sus campos de entrada.
func (h *ModulesHandler) ExecAction(c *gin.Context) {
	def, err := h.repo.FindBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, modules.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo cargar el módulo")
		return
	}

	actionID := c.Param("actionId")
	act, ok := findAction(def.Actions, actionID)
	if !ok {
		writeError(c, http.StatusNotFound, "E404", "Acción no encontrada en el módulo")
		return
	}

	var input map[string]any
	if err := c.ShouldBindJSON(&input); err != nil {
		input = map[string]any{}
	}
	// Para acciones de editar/eliminar de un módulo Postgres cuya definición ya
	// declara columnas llave primaria, el formulario (y por tanto el input) se
	// genera DESDE LAS COLUMNAS (Opción B), no desde inputFields. En ese caso la
	// validación de entrada debe hacerse contra los FieldSpec derivados de las
	// columnas (PK obligatorias + editables), no contra act.InputFields. Si la
	// definición es antigua y no declara PK de columna, se valida contra
	// inputFields (fallback para módulos no re-guardados). El resto de acciones
	// (listar/crear/custom) se valida contra inputFields sin cambios.
	validateFields := act.InputFields
	if def.SourceType == modules.SourcePostgres &&
		(act.Kind == modules.ActionKindEdit || act.Kind == modules.ActionKindDelete) {
		// Resuelve el discriminante multi-tabla UNA vez y lo pasa a EditFieldSpecs
		// para que la clave validada (alias <T>__<k> en multi-tabla, pelada en N=1)
		// sea la MISMA que envía el frontend y lee el motor (Opción A). La red
		// PRINCIPAL de PK faltante es este handler con E422_VALIDATION; la fase de
		// aborto del motor (E502 table-qualified) es la SEGUNDA red.
		spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
		cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, spec.PrincipalTable)
		if len(modules.PrimaryKeyColumns(cols)) > 0 {
			validateFields = modules.EditFieldSpecs(cols, act, spec)
		}
	}
	// Acción INSERT (create o custom) con estrategias de llenado: se valida contra
	// los campos derivados (solo presencia de las capturas; derive-*/constant/
	// autogenerated no los teclea el usuario). Se ramifica por SQLOp (no por Kind)
	// para cubrir también acciones custom con SQLOp=INSERT.
	if def.SourceType == modules.SourcePostgres &&
		strings.ToUpper(strings.TrimSpace(act.SQLOp)) == "INSERT" &&
		modules.HasFillStrategies(act) {
		validateFields = modules.CreateFieldSpecs(act)
	}
	if err := engine.ValidateInput(validateFields, input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "Los campos de entrada no son válidos: "+err.Error())
		return
	}

	exec, err := h.executorFor(*def)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "Tipo de origen no soportado")
		return
	}

	country, _ := CountryFromContext(c)
	result, objectID, err := exec.Exec(c.Request.Context(), *def, country, act, input)
	if err != nil {
		if errors.Is(err, engine.ErrWhitelist) {
			writeError(c, http.StatusUnprocessableEntity, "E422_PG_WHITELIST", "La operación referencia tablas o columnas no permitidas")
			return
		}
		if errors.Is(err, engine.ErrDeriveRelation) {
			// Mensaje FIJO de negocio: no se concatena err.Error() para no filtrar
			// nombres de tabla/columna ni el nº de filas (el detalle queda en logs).
			writeError(c, http.StatusUnprocessableEntity, "E422_DERIVE", "No se pudo resolver la relación para la opción elegida")
			return
		}
		if errors.Is(err, engine.ErrUniqueness) {
			// Conflicto de unicidad: mensaje de NEGOCIO del admin (nunca SQL). Se toma
			// del propio Uniqueness.Message de la acción; vacío ⇒ genérico en español.
			msg := "Ya existe un registro con esa combinación de valores"
			if act.Uniqueness != nil && strings.TrimSpace(act.Uniqueness.Message) != "" {
				msg = act.Uniqueness.Message
			}
			writeError(c, http.StatusConflict, "E409_UNIQUENESS", msg)
			return
		}
		writeError(c, http.StatusBadGateway, "E502", "El origen del módulo no pudo ejecutar la acción: "+err.Error())
		return
	}
	if objectID != "" {
		SetAuditObjectID(c, objectID)
	}
	writeOK(c, http.StatusOK, result, "")
}

// ExecActionList maneja POST /modules/:slug/actions/:actionId/list: ejecuta una
// acción SELECT como una LECTURA (vuelve a correr el listado del módulo con los
// campos LLENOS de la acción inyectados como WHERE adicional). Es una ruta
// dedicada de lectura (permiso `ver`, auditoría solo en error), NO la vía de
// escritura de ExecAction. La paginación/orden viaja por la query string y los
// valores de filtro por el cuerpo JSON.
func (h *ModulesHandler) ExecActionList(c *gin.Context) {
	def, err := h.repo.FindBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, modules.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Módulo no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo cargar el módulo")
		return
	}

	// La acción se empareja por ActionKey aunque el parámetro de ruta se llame
	// :actionId (coherente con ExecAction).
	act, ok := findAction(def.Actions, c.Param("actionId"))
	if !ok {
		writeError(c, http.StatusNotFound, "E404", "Acción no encontrada en el módulo")
		return
	}

	// Paginación/orden desde la query; los valores de filtro desde el cuerpo JSON.
	q := parseListQuery(c)
	var input map[string]any
	if err := c.ShouldBindJSON(&input); err != nil {
		input = map[string]any{}
	}

	// Validación de los campos como OPCIONALES: se copian los InputFields con
	// Required=false para que un filtro vacío NUNCA bloquee; solo se valida por
	// tipo cuando hay valor (comportamiento de engine.ValidateInput).
	optionalFields := make([]modules.FieldSpec, len(act.InputFields))
	copy(optionalFields, act.InputFields)
	for i := range optionalFields {
		optionalFields[i].Required = false
	}
	if err := engine.ValidateInput(optionalFields, input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "Los campos de entrada no son válidos: "+err.Error())
		return
	}

	// El filtrado de lista vive en *PGExecutor (como FieldOptions). Se asegura el
	// tipo de origen y el tipo concreto.
	if _, err := h.executorFor(*def); err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "Tipo de origen no soportado")
		return
	}
	pg, ok := h.pg.(*engine.PGExecutor)
	if !ok {
		writeError(c, http.StatusInternalServerError, "E500", "El ejecutor del módulo no soporta el filtrado por acción")
		return
	}

	country, _ := CountryFromContext(c)
	rows, page, err := pg.ListFiltered(c.Request.Context(), *def, country, q, act, input)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrWhitelist):
			// Mensaje NEUTRO (igual que List): el origen concreto queda solo en logs.
			writeError(c, http.StatusUnprocessableEntity, "E422_PG_WHITELIST",
				"La solicitud referencia tablas o columnas no permitidas")
		default:
			writeError(c, http.StatusBadGateway, "E502",
				"El origen del módulo no respondió correctamente: "+err.Error())
		}
		return
	}
	// Misma forma de respuesta que List (envoltorio ok/data/message).
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"items": rows,
			"pagination": gin.H{
				"totalRecords":   page.TotalRecords,
				"page":           page.Page,
				"recordsPerPage": page.RecordsPerPage,
				"totalPages":     page.TotalPages,
			},
		},
		"message": "",
	})
}

// findAction localiza una acción por su action_key.
func findAction(actions []modules.Action, key string) (modules.Action, bool) {
	for _, a := range actions {
		if a.ActionKey == key {
			return a, true
		}
	}
	return modules.Action{}, false
}

// parseListQuery lee los parámetros de lista de la query (paginación/orden). Lo
// reutilizan GET /list y la ruta dedicada POST /actions/:actionId/list; en esta
// última los valores de filtro NO viajan por la query sino por el cuerpo JSON.
func parseListQuery(c *gin.Context) engine.ListQuery {
	page, _ := strconv.Atoi(strings.TrimSpace(c.Query("page")))
	per, _ := strconv.Atoi(strings.TrimSpace(c.Query("records_per_page")))
	return engine.ListQuery{
		Page:           page,
		RecordsPerPage: per,
		OrderBy:        strings.TrimSpace(c.Query("order_by")),
		Order:          strings.TrimSpace(c.Query("order")),
	}
}
