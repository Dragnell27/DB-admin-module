package connections

import (
	"encoding/json"
	"time"
)

// templateRow mapea la tabla `connection_templates` (migración 000008). El campo
// `fields` guarda la ESTRUCTURA de los campos (JSONB, NO sensible); `secrets_enc`
// es el bundle cifrado de valores FIJOS (BYTEA, AES-GCM). Los ids los genera la
// BD con gen_random_uuid() cuando no se asignan.
type templateRow struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Name       string    `gorm:"column:name;type:varchar(150);uniqueIndex;not null"`
	Fields     []byte    `gorm:"column:fields;type:jsonb;not null;default:'[]'"`
	SecretsEnc []byte    `gorm:"column:secrets_enc;type:bytea"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (templateRow) TableName() string {
	return "connection_templates"
}

// encodeFields serializa la estructura de campos a JSONB. Nunca contiene valores
// sensibles (solo claves/marcas/roles/sugeridos).
func encodeFields(fields []TemplateField) []byte {
	if len(fields) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// decodeFields deserializa la estructura de campos desde JSONB.
func decodeFields(b []byte) []TemplateField {
	if len(b) == 0 {
		return nil
	}
	var fields []TemplateField
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil
	}
	return fields
}
