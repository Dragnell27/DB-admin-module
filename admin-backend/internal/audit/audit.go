// Package audit implementa la bitácora de auditoría del panel: el registro de
// las operaciones de escritura (quién, qué, sobre qué objeto, cuándo, país y
// resultado) y su consulta filtrada. La persistencia es en PostgreSQL (tabla
// audit_logs, migración 000004).
//
// El registro es un EFECTO SECUNDARIO best-effort: un fallo al insertar una
// entrada nunca debe hacer fallar la operación principal (lo decide el llamador,
// típicamente el middleware de auditoría, que solo loguea el error).
package audit

import (
	"context"
	"time"
)

// Resultados posibles de una operación auditada.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// Entry es una entrada de la bitácora: una operación de escritura registrada.
type Entry struct {
	ID         string                 `json:"id"`
	ActorID    string                 `json:"actorId"`
	ActorName  string                 `json:"actorName"`
	Resource   string                 `json:"resource"`
	Action     string                 `json:"action"`
	ObjectID   string                 `json:"objectId"`
	Country    string                 `json:"country"`
	Outcome    string                 `json:"outcome"`
	StatusCode int                    `json:"statusCode"`
	ErrorCode  string                 `json:"errorCode"`
	Message    string                 `json:"message"`
	Payload    map[string]interface{} `json:"payload"`
	CreatedAt  time.Time              `json:"createdAt"`
}

// Filter describe los criterios de consulta de la bitácora. Todos los campos de
// filtro son opcionales y combinables; Page/PerPage controlan la paginación.
type Filter struct {
	// ObjectID filtra por el id exacto del objeto afectado (filtro principal).
	ObjectID string
	// Resource filtra por recurso (customers/plans/...).
	Resource string
	// Actor filtra por actor: coincide con actor_name o actor_id.
	Actor string
	// Action filtra por acción (crear/editar/...).
	Action string
	// From/To acotan por rango de fechas de created_at (inclusive).
	From *time.Time
	To   *time.Time
	// Paginación (1-based). Valores no positivos los normaliza el repositorio.
	Page    int
	PerPage int
}

// Repository define la persistencia de la bitácora.
type Repository interface {
	// Insert registra una entrada. Devuelve error si no pudo persistir; el
	// llamador decide si lo propaga o solo lo loguea (best-effort).
	Insert(ctx context.Context, e Entry) error
	// List devuelve las entradas que satisfacen el filtro, ordenadas por fecha
	// descendente (más recientes primero), junto con el total que coincide con
	// el filtro (para la paginación).
	List(ctx context.Context, f Filter) ([]Entry, int64, error)
}
