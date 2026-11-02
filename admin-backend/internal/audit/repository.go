package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// auditRow es el modelo GORM que mapea la tabla `audit_logs`. El payload se
// persiste como texto JSON en la columna JSONB (el driver de Postgres acepta un
// string JSON válido para una columna jsonb). Se mantiene separado del modelo de
// dominio Entry para aislar la (de)serialización del payload.
type auditRow struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ActorID    *string   `gorm:"column:actor_id;type:uuid"`
	ActorName  string    `gorm:"column:actor_name;type:varchar(100);not null"`
	Resource   string    `gorm:"column:resource;type:varchar(32);not null"`
	Action     string    `gorm:"column:action;type:varchar(32);not null"`
	ObjectID   string    `gorm:"column:object_id;type:varchar(128)"`
	Country    string    `gorm:"column:country;type:varchar(8)"`
	Outcome    string    `gorm:"column:outcome;type:varchar(16);not null"`
	StatusCode int       `gorm:"column:status_code;not null"`
	ErrorCode  string    `gorm:"column:error_code;type:varchar(64)"`
	Message    string    `gorm:"column:message;type:text"`
	Payload    *string   `gorm:"column:payload;type:jsonb"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (auditRow) TableName() string {
	return "audit_logs"
}

// gormRepository es la implementación de Repository respaldada por GORM.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository crea un Repository de auditoría sobre la conexión GORM dada.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// Insert persiste una entrada de auditoría. El payload se serializa a JSON; si
// la serialización falla, se guarda sin payload (no se pierde el resto de la
// entrada).
func (r *gormRepository) Insert(ctx context.Context, e Entry) error {
	row := auditRow{
		ID:         strings.TrimSpace(e.ID),
		ActorName:  e.ActorName,
		Resource:   e.Resource,
		Action:     e.Action,
		ObjectID:   e.ObjectID,
		Country:    strings.ToLower(strings.TrimSpace(e.Country)),
		Outcome:    e.Outcome,
		StatusCode: e.StatusCode,
		ErrorCode:  e.ErrorCode,
		Message:    e.Message,
	}
	if id := strings.TrimSpace(e.ActorID); id != "" {
		row.ActorID = &id
	}
	if len(e.Payload) > 0 {
		if b, err := json.Marshal(e.Payload); err == nil {
			s := string(b)
			row.Payload = &s
		}
	}

	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("audit: no se pudo insertar la entrada: %w", err)
	}
	return nil
}

// List devuelve las entradas que satisfacen el filtro, ordenadas por created_at
// descendente, junto con el total que coincide (para la paginación).
func (r *gormRepository) List(ctx context.Context, f Filter) ([]Entry, int64, error) {
	q := r.db.WithContext(ctx).Model(&auditRow{})

	if v := strings.TrimSpace(f.ObjectID); v != "" {
		q = q.Where("object_id = ?", v)
	}
	if v := strings.TrimSpace(f.Resource); v != "" {
		q = q.Where("resource = ?", v)
	}
	if v := strings.TrimSpace(f.Action); v != "" {
		q = q.Where("action = ?", v)
	}
	if v := strings.TrimSpace(f.Actor); v != "" {
		// Coincide por nombre o por id del actor.
		q = q.Where("actor_name = ? OR actor_id::text = ?", v, v)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("audit: error contando entradas: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var rows []auditRow
	if err := q.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("audit: error listando entradas: %w", err)
	}

	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, toEntry(row))
	}
	return entries, total, nil
}

// toEntry convierte una fila de la tabla al modelo de dominio, deserializando el
// payload JSON si está presente.
func toEntry(row auditRow) Entry {
	e := Entry{
		ID:         row.ID,
		ActorName:  row.ActorName,
		Resource:   row.Resource,
		Action:     row.Action,
		ObjectID:   row.ObjectID,
		Country:    row.Country,
		Outcome:    row.Outcome,
		StatusCode: row.StatusCode,
		ErrorCode:  row.ErrorCode,
		Message:    row.Message,
		CreatedAt:  row.CreatedAt,
	}
	if row.ActorID != nil {
		e.ActorID = *row.ActorID
	}
	if row.Payload != nil && *row.Payload != "" {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(*row.Payload), &m); err == nil {
			e.Payload = m
		}
	}
	return e
}
