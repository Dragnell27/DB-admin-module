// Package modules define el modelo de dominio del constructor de módulos
// (module-builder) y el repositorio de acceso a datos sobre las tablas
// `module_definitions`, `module_data_sources` y `module_actions`
// (PostgreSQL / GORM, migración 000005).
//
// Una Definición de módulo agrupa: metadatos (slug, nombre, tipo de origen,
// ámbito de país), un Origen de datos PostgreSQL (con credenciales CIFRADAS en
// reposo), mapeos flexibles en JSONB (obtención/visualización/parámetros de
// lista) y un conjunto de Acciones (CRUD + personalizadas).
//
// Las credenciales sensibles se cifran con internal/secrets antes de persistir
// y NUNCA se exponen al frontend: el modelo de dominio público las omite por
// construcción (ver DataSource, que no incluye los secretos descifrados).
package modules

import (
	"errors"
	"time"
)

// Tipos de origen de datos soportados (Req. 4). El único origen del
// constructor es PostgreSQL; el origen "API HTTP" fue retirado.
const (
	SourcePostgres = "postgres"
)

// Ámbitos de país de un módulo (Req. 11.7).
const (
	CountryScopeNone  = "none"  // módulo sin país (único)
	CountryScopeMulti = "multi" // módulo por país
)

// Tipos de acción (Req. 7.1).
const (
	ActionKindList   = "list"
	ActionKindCreate = "create"
	ActionKindEdit   = "edit"
	ActionKindDelete = "delete"
	ActionKindCustom = "custom"
	// ActionKindSearch es una lectura (SELECT) con filtro, hermana de 'list':
	// no escribe (cae en default de IsWriteKind -> false), RBAC la deriva por
	// PermAction y findListAction NO la empareja (solo Kind==list alimenta la
	// tabla BASE). Añadida de forma aditiva; la columna kind es varchar libre.
	ActionKindSearch = "search"
)

// Ubicación del botón de una acción (Req. 8.1).
const (
	PlacementRow    = "row"
	PlacementHeader = "header"
)

// Orígenes del valor de un campo de entrada (FieldSpec.Source). Hoy coexisten
// cuatro: "" / "input" (lo captura el usuario), "row" (de la fila), "constant"
// (literal fijo) y "select" (lista estática de opciones etiqueta/valor definida
// en el builder; el usuario elige una en tiempo de ejecución).
const (
	FieldSourceInput    = "input"
	FieldSourceRow      = "row"
	FieldSourceConstant = "constant"
	FieldSourceSelect   = "select"
	// FieldSourceDynamic marca un campo cuyas opciones (etiqueta/valor) se
	// obtienen EN TIEMPO DE EJECUCIÓN de otra tabla de la MISMA conexión del
	// módulo (ver DynamicSource). El usuario elige una opción; la petición envía
	// el id (clave foránea) de la opción elegida.
	FieldSourceDynamic = "dynamic"
)

// Errores de negocio exportados del repositorio.
var (
	// ErrNotFound se devuelve cuando no existe un módulo que satisfaga la consulta.
	ErrNotFound = errors.New("modules: módulo no encontrado")
	// ErrSlugTaken se devuelve al crear un módulo con un slug ya existente
	// (slug único = recurso dinámico único, Req. 1.3).
	ErrSlugTaken = errors.New("modules: el slug del módulo ya está en uso")
	// ErrInvalidSlug se devuelve cuando el slug no es kebab-case válido (Req. 1.2).
	ErrInvalidSlug = errors.New("modules: el slug del módulo es inválido (se espera kebab-case)")
	// ErrEmptyName se devuelve cuando el nombre del módulo está vacío.
	ErrEmptyName = errors.New("modules: el nombre del módulo no puede estar vacío")
	// ErrTemplateRequired se devuelve cuando el módulo no referencia una
	// plantilla de conexión válida (Req. 7.1): la plantilla es el ÚNICO
	// mecanismo de conexión.
	ErrTemplateRequired = errors.New("modules: el módulo debe referenciar una plantilla de conexión existente")
)

// SelectOption es una opción de un campo de tipo "select": una ETIQUETA visible
// y el VALOR que se envía al elegirla. La etiqueta puede diferir del valor
// (p. ej. etiqueta "Guatemala", valor "gt"). Opciones ESTÁTICAS definidas en el
// builder; se deja la puerta abierta a opciones dinámicas en el futuro sin
// cambiar esta forma.
type SelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// OrderSpec describe el ordenamiento opcional de las opciones de un origen
// dinámico: la columna por la que ordenar y la dirección ("asc" | "desc").
// Column vacío significa "sin ORDER BY".
type OrderSpec struct {
	Column    string `json:"column,omitempty"`
	Direction string `json:"direction,omitempty"` // asc|desc
}

// DynamicSource describe de dónde se obtienen las opciones (etiqueta/valor) de
// un campo con Source=="dynamic": una tabla de la MISMA conexión del módulo, la
// columna que aporta el id (clave foránea que se guarda) y la columna que aporta
// la etiqueta visible. Filtros opcionales (reutilizando ListCondition, se
// traducen al mismo WHERE parametrizado y gateado por lista blanca), orden
// opcional y un tope de filas. Tabla y columnas se validan contra la lista
// blanca del módulo ANTES de componer SQL.
type DynamicSource struct {
	Table       string          `json:"table"`
	IDColumn    string          `json:"idColumn"`
	LabelColumn string          `json:"labelColumn"`
	Filters     []ListCondition `json:"filters,omitempty"`
	Order       *OrderSpec      `json:"order,omitempty"`
	Limit       int             `json:"limit,omitempty"`
}

// FieldSpec describe un campo de entrada de una Acción: su nombre, tipo y
// obligatoriedad (Req. 7.5, 10.3). Se serializa como parte de input_fields
// (JSONB). El valor del campo puede provenir del usuario, de la fila
// seleccionada, ser un valor constante fijo definido al crear el módulo
// (p. ej. un valor "gt") o elegirse de una lista estática de opciones.
//
// INVARIANTE para ACCIONES DE ESCRITURA Postgres (create/edit): cada Name DEBE
// ser el nombre de una COLUMNA de la tabla (DisplayColumn.Field). El Source
// indica el MODO de esa columna objetivo: "input" (capturado), "constant" (valor
// fijo, posiblemente oculto en el formulario) o "select" (lista estática). Así,
// una acción de escritura es una operación ESPECIALIZADA que toca un subconjunto
// de columnas editables, cada una con su origen de valor; el backend impone que
// solo se escriban columnas editables no-PK (ver ActionWriteColumns). El origen
// "row" NO se usa para escrituras Postgres: la PK del WHERE se toma de la fila
// por el camino de columnas, no por un campo. Para acciones custom o para
// orígenes no Postgres, Name sigue siendo el nombre libre del parámetro.
type FieldSpec struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string | number | boolean | date
	Required bool   `json:"required"`
	// Source indica el origen del valor del campo: "input" (lo captura el
	// usuario), "row" (se toma de un campo de la fila, mapeo explícito) o
	// "constant" (valor literal fijo definido al crear el módulo; ver
	// ConstValue). Vacío equivale a "input" por compatibilidad.
	Source string `json:"source,omitempty"`
	// RowField es, cuando Source == "row", el nombre del campo de la fila del
	// que se toma el valor (p. ej. el parámetro "plan_id" se llena con "id").
	RowField string `json:"rowField,omitempty"`
	// ConstValue es, cuando Source == "constant", el valor literal fijo que se
	// envía siempre (p. ej. "gt" para un header "country"), sin leerlo del input.
	ConstValue string `json:"constValue,omitempty"`
	// IsKey marca, en Postgres UPDATE/DELETE, que este campo forma parte de la
	// clave del WHERE. Varios campos marcados forman una clave compuesta. El
	// Name del campo debe ser el nombre de la columna real.
	//
	// DEPRECADO: la llave primaria pasó a declararse A NIVEL DE COLUMNA
	// (DisplayColumn.IsPrimaryKey, ver column.go). El constructor ya no produce
	// este campo. Se conserva ÚNICAMENTE para (1) parsear definiciones JSONB
	// antiguas sin error y (2) servir de fallback de EJECUCIÓN cuando una
	// definición aún no re-guardada no declara columnas PK (ver
	// engine.pg_executor.legacyKeyColumns). No usar en código nuevo.
	IsKey bool `json:"isKey,omitempty"`
	// Options es, cuando Source == "select", la lista ESTÁTICA de opciones
	// (etiqueta/valor) que el usuario puede elegir. El desplegable muestra las
	// etiquetas; la petición envía el Value de la opción elegida.
	Options []SelectOption `json:"options,omitempty"`
	// DefaultValue es, cuando Source == "select", el VALUE de la opción
	// preseleccionada por defecto. Si el usuario no cambia la selección, se
	// envía este valor. Debe coincidir con el Value de alguna opción de Options.
	DefaultValue string `json:"defaultValue,omitempty"`
	// DynamicSource es, cuando Source == "dynamic", el origen desde el que se
	// obtienen en tiempo de ejecución las opciones (etiqueta/valor) del campo.
	// El desplegable muestra la etiqueta; la petición envía el id (FK) elegido.
	DynamicSource *DynamicSource `json:"dynamicSource,omitempty"`
	// SourceTable es, en módulos MULTI-TABLA, la tabla a la que pertenece la
	// columna OBJETIVO de una acción edit/create cuando su nombre es homónimo
	// entre varias tablas (desambigua el emparejamiento campo→columna por
	// (sourceTable, field)). SOLO cualifica columnas objetivo
	// (input/constant/select/dynamic); NUNCA se usa para una PK ni para un campo
	// con Source=="row" (la PK del WHERE viaja por el camino de columnas aliased
	// <tabla>__<col>). Ausente ⇒ la tabla principal (compatibilidad N=1).
	SourceTable string `json:"sourceTable,omitempty"`
	// MatchType es, para un campo de una acción SELECT (filtro de listado), el
	// tipo de coincidencia: "exact" (lower(btrim(col::text)) = $n) o "partial"
	// (lower(col::text) LIKE '%'||lower($n)||'%', patrón con parámetro ligado y
	// comodines literales). Ausente ⇒ "exact". Solo tiene efecto en acciones
	// SELECT; para INSERT/UPDATE/DELETE se ignora.
	MatchType string `json:"matchType,omitempty"`
}

// Action es el modelo de dominio de una acción del módulo (una fila de
// `module_actions`). El origen PostgreSQL usa SQLOp. InputFields define el
// formulario autogenerado.
type Action struct {
	ID          string      `json:"id"`
	ActionKey   string      `json:"actionKey"`   // estable por módulo (p. ej. "list", "custom:activar")
	Kind        string      `json:"kind"`        // list|create|edit|delete|custom
	Label       string      `json:"label"`       // etiqueta del botón
	Placement   string      `json:"placement"`   // row|header
	Destructive bool        `json:"destructive"` // confirma antes de ejecutar
	PermAction  string      `json:"permAction"`  // acción de permiso RBAC (ver/crear/editar/eliminar/custom:*)
	IsWrite     bool        `json:"isWrite"`     // determina auditoría de escritura
	SQLOp       string      `json:"sqlOp,omitempty"`
	InputFields []FieldSpec `json:"inputFields"`
	// DeleteTopTable es, para una acción DELETE MULTI-TABLA, la "tabla tope"
	// hasta la que borra. El conjunto ordenado hijo→padre se DERIVA del árbol de
	// relaciones (no se teclea a mano). Ausente ⇒ la tabla principal (N=1: borra
	// solo la principal, como hoy).
	DeleteTopTable string `json:"deleteTopTable,omitempty"`
	// TargetTable es, para una acción INSERT (create) MULTI-TABLA, la tabla
	// objetivo del INSERT. Ausente ⇒ la tabla principal (N=1, sin regresión).
	TargetTable string `json:"targetTable,omitempty"`
}

// DataSource es el modelo de dominio del origen de datos de un módulo, SIN las
// credenciales descifradas: expone solo la configuración no sensible. El
// secreto (DSN efectivo de PostgreSQL) nunca sale del backend (Req. 4.3, 13.3).
type DataSource struct {
	SourceType  string              `json:"sourceType"`            // postgres
	PGWhitelist map[string][]string `json:"pgWhitelist,omitempty"` // Postgres: tabla -> columnas permitidas
	// ConnectionTemplateID es la FK a la plantilla de conexión que usa el módulo
	// (connection-templates). Cadena vacía si el módulo aún no referencia una
	// plantilla (solo durante la ventana de migración).
	ConnectionTemplateID string `json:"connectionTemplateId"`
	// HasSecrets indica si el origen tiene valores VARIABLE cifrados almacenados,
	// sin revelar su valor (para que la UI muestre "configurado / sin cambios").
	HasSecrets bool `json:"hasSecrets"`
}

// ListCondition es una condición fija de listado: solo pasan las filas cuya
// columna `Field` tiene un valor que coincide (exacto, sin distinguir
// mayúsculas) con alguno de `Values`. Varias condiciones se combinan con AND.
//
// El filtrado se aplica 100% EN SQL vía engine.buildListConditionsWhere, con la
// MISMA cláusula WHERE y argumentos ligados para el COUNT(*) y el SELECT de
// datos (paginación consistente). En módulos MULTI-TABLA cada condición se
// cualifica por su tabla (lower(btrim("<tabla>"."<col>"::text)) IN (...)).
// engine.applyConditions/rowMeetsAll (internal/engine/mapping.go) quedan
// INACTIVOS (no se invocan en el camino en vivo) y NO deben reintroducirse sobre
// filas aliased <tabla>__<col> (indexan por nombre PELADO y descartarían todas
// las filas).
type ListCondition struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
	// SourceTable es, en módulos MULTI-TABLA, la tabla de la columna `Field`
	// cuando el nombre es homónimo entre tablas. Ausente ⇒ la principal (o la
	// única tabla que declara esa columna). Si la columna es homónima y no se
	// declara, el guardado se rechaza (validateMultiTable).
	SourceTable string `json:"sourceTable,omitempty"`
}

// Definition es el modelo de dominio (representación pública) de un módulo
// completo: metadatos + origen (sin secretos) + mapeos + acciones.
type Definition struct {
	ID             string          `json:"id"`
	Slug           string          `json:"slug"`
	Name           string          `json:"name"`
	SourceType     string          `json:"sourceType"`
	CountryScope   string          `json:"countryScope"`
	FetchMapping   map[string]any  `json:"fetchMapping"`
	DisplayMapping map[string]any  `json:"displayMapping"`
	ListParams     map[string]any  `json:"listParams"`
	ListConditions []ListCondition `json:"listConditions"`
	DataSource     DataSource      `json:"dataSource"`
	MenuPath       string          `json:"menuPath"`
	Actions        []Action        `json:"actions"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// Secrets agrupa los valores en claro de un origen al CREAR/EDITAR un módulo: los
// valores VARIABLE que el módulo aporta a su plantilla de conexión. El
// repositorio los cifra con internal/secrets antes de persistir y nunca los
// devuelve.
//
// Semántica "sin cambios": un VariableValues ausente o `{}` significa "sin
// cambios" en TODAS las claves (conserva el bundle previo íntegro); NO existe
// forma de vaciar una variable por esta ruta (coherente con el patrón escalar
// previo donde DSN == "" significaba "sin cambios").
type Secrets struct {
	VariableValues map[string]string `json:"variableValues,omitempty"`
	// DSN es el DSN EFECTIVO ya ensamblado en memoria que devuelve RevealSecrets
	// para que el executor (engine/pg_executor.go) lo abra sin cambios en su
	// firma. NUNCA se acepta desde el cliente ni se persiste: se ignora al
	// serializar el cuerpo de crear/editar (no lleva tag json de entrada) y solo
	// lo rellena RevealSecrets en el camino de ejecución.
	DSN string `json:"-"`
}

// IsWriteKind indica si un tipo de acción corresponde a una escritura (crea,
// edita, elimina o personalizada marcada como escritura). Las acciones Listar
// son de solo lectura.
func IsWriteKind(kind string) bool {
	switch kind {
	case ActionKindCreate, ActionKindEdit, ActionKindDelete:
		return true
	default:
		return false
	}
}
