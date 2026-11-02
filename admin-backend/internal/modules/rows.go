package modules

import "time"

// Modelos GORM que mapean las tablas de la migración 000005. Los campos JSONB se
// almacenan como []byte (serializados con encoding/json en el repositorio) para
// no añadir la dependencia gorm.io/datatypes; secretsEnc es BYTEA (credenciales
// cifradas con internal/secrets). Los ids los genera la BD con gen_random_uuid()
// cuando no se asignan.

// definitionRow mapea la tabla `module_definitions`.
type definitionRow struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug           string    `gorm:"column:slug;type:varchar(64);uniqueIndex;not null"`
	Name           string    `gorm:"column:name;type:varchar(150);not null"`
	SourceType     string    `gorm:"column:source_type;type:varchar(16);not null"`
	CountryScope   string    `gorm:"column:country_scope;type:varchar(16);not null;default:none"`
	MenuPath       string    `gorm:"column:menu_path;type:varchar(512);not null;default:''"`
	FetchMapping   []byte    `gorm:"column:fetch_mapping;type:jsonb;not null;default:'{}'"`
	DisplayMapping []byte    `gorm:"column:display_mapping;type:jsonb;not null;default:'{}'"`
	ListParams     []byte    `gorm:"column:list_params;type:jsonb;not null;default:'{}'"`
	ListConditions []byte    `gorm:"column:list_conditions;type:jsonb;not null;default:'[]'"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (definitionRow) TableName() string {
	return "module_definitions"
}

// dataSourceRow mapea la tabla `module_data_sources` (1:1 con el módulo).
// SecretsEnc contiene las credenciales cifradas (BYTEA); puede ser nil.
type dataSourceRow struct {
	ID          string `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ModuleID    string `gorm:"column:module_id;type:uuid;uniqueIndex;not null"`
	SourceType  string `gorm:"column:source_type;type:varchar(16);not null"`
	SecretsEnc  []byte `gorm:"column:secrets_enc;type:bytea"`
	PGWhitelist []byte `gorm:"column:pg_whitelist;type:jsonb;not null;default:'{}'"`
	// ConnectionTemplateID es la FK a connection_templates (nullable durante la
	// ventana de migración). GORM solo lee/escribe columnas mapeadas: sin este
	// campo, el SELECT/INSERT ignoraría la columna y la FK nunca se escribiría.
	ConnectionTemplateID *string `gorm:"column:connection_template_id;type:uuid"`
	// SecretsEncLegacy respalda el DSN cifrado legado durante la migración
	// reversible (Req. 7.5). El dominio nunca lo escribe/limpia: solo lo arrastra.
	SecretsEncLegacy []byte    `gorm:"column:secrets_enc_legacy;type:bytea"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt        time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (dataSourceRow) TableName() string {
	return "module_data_sources"
}

// actionRow mapea la tabla `module_actions`.
type actionRow struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ModuleID    string    `gorm:"column:module_id;type:uuid;not null"`
	ActionKey   string    `gorm:"column:action_key;type:varchar(64);not null"`
	Kind        string    `gorm:"column:kind;type:varchar(16);not null"`
	Label       string    `gorm:"column:label;type:varchar(120);not null"`
	Placement   string    `gorm:"column:placement;type:varchar(8);not null"`
	Destructive bool      `gorm:"column:destructive;not null;default:false"`
	PermAction  string    `gorm:"column:perm_action;type:varchar(32);not null"`
	IsWrite     bool      `gorm:"column:is_write;not null;default:false"`
	SQLOp       string    `gorm:"column:sql_op;type:varchar(8)"`
	InputFields []byte    `gorm:"column:input_fields;type:jsonb;not null;default:'[]'"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (actionRow) TableName() string {
	return "module_actions"
}
