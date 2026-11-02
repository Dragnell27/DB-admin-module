// Package engine implementa el motor de ejecución del constructor de módulos
// (module-builder): el componente del admin-backend que, a partir de una
// Definición de módulo, ejecuta la Acción Listar y las Acciones contra el
// origen de datos PostgreSQL. Es el ÚNICO ejecutor: el frontend nunca llama a
// orígenes externos (Req. 13.1, 13.2).
//
// Expone una interfaz común Executor cuya implementación es el Postgres executor
// (consultas parametrizadas + lista blanca). El router dinámico selecciona el
// ejecutor por el tipo de origen del módulo.
package engine

import (
	"context"

	"admin-backend/internal/modules"
)

// ListQuery agrupa los parámetros de la Acción Listar resueltos desde la
// petición: paginación y orden. El motor los traduce a los parámetros
// declarados del origen (list_params) o los emula en memoria cuando el origen
// no los soporta (Req. 9.3, 9.4, 9.5). Los valores de filtro de una acción
// SELECT NO viajan aquí: llegan por el cuerpo JSON de la ruta dedicada de
// lectura y se aplican vía ListFiltered.
type ListQuery struct {
	Page           int
	RecordsPerPage int
	OrderBy        string
	Order          string // "asc" | "desc"
}

// Pagination son los metadatos de paginación devueltos por una Acción Listar,
// con la misma forma que el envoltorio uniforme del BFF.
type Pagination struct {
	TotalRecords   int64 `json:"totalRecords"`
	Page           int   `json:"page"`
	RecordsPerPage int   `json:"recordsPerPage"`
	TotalPages     int   `json:"totalPages"`
}

// Executor abstrae la ejecución de un módulo contra su origen. List produce la
// colección mostrada en la tabla (ya mapeada por el Display_Mapping); Exec
// ejecuta una Acción (crear/editar/eliminar/personalizada) con los campos de
// entrada ya validados, devolviendo el resultado y el objectId afectado (para
// la auditoría).
type Executor interface {
	List(ctx context.Context, def modules.Definition, country string, q ListQuery) (rows []map[string]any, page Pagination, err error)
	Exec(ctx context.Context, def modules.Definition, country string, act modules.Action, input map[string]any) (result any, objectID string, err error)
}

// SecretProvider descifra en memoria las credenciales del origen de un módulo
// para que el motor las use en el momento de ejecutar. Lo implementa el
// repositorio de módulos (método RevealSecrets); nunca expone secretos al
// frontend.
type SecretProvider interface {
	RevealSecrets(ctx context.Context, slug string) (modules.Secrets, error)
}

// PoolInvalidator permite invalidar el pool cacheado de un módulo para que la
// próxima ejecución reabra con el DSN efectivo recalculado (D-4, Req. 3.5).
// Es una interfaz acotada, ortogonal a Executor (List/Exec): no se mezcla la
// preocupación de ciclo de vida del pool con la de ejecución. El tipo concreto
// *PGExecutor la satisface automáticamente.
type PoolInvalidator interface {
	Invalidate(slug string)
}
