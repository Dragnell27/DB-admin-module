// Package connections define el dominio de las plantillas de conexión
// (connection-templates) y el ensamblado del DSN efectivo en memoria.
//
// Una plantilla reutilizable define un conjunto de campos (host, puerto,
// usuario, password, nombreBD…), cada uno marcado FIJO (valor constante guardado
// cifrado en la plantilla) o VARIABLE (lo aporta cada módulo). Un módulo de BD
// referencia una plantilla y aporta sus valores VARIABLE (cifrados en su
// origen). El DSN efectivo se ensambla EN MEMORIA al ejecutar combinando los
// campos FIJOS de la plantilla con los VARIABLE del módulo.
//
// INVARIANTE DE ARQUITECTURA: este paquete NO importa internal/modules ni
// internal/engine. Depende solo de internal/secrets, gorm y la stdlib. El grafo
// de dependencias es dirigido y acíclico: internal/modules importa
// internal/connections, nunca al revés (hallazgo 7). El conteo UsedByModules y
// el bloqueo de borrado se resuelven por consulta SQL directa a la tabla
// module_data_sources (que es una tabla, no un paquete).
package connections

import (
	"errors"
	"time"
)

// FieldKind marca un campo de la plantilla como FIJO o VARIABLE.
type FieldKind string

const (
	// FieldFixed marca un campo cuyo valor es constante y vive cifrado en la
	// plantilla (secrets_enc).
	FieldFixed FieldKind = "fixed"
	// FieldVariable marca un campo cuyo valor lo aporta cada módulo al referenciar
	// la plantilla (cifrado en module_data_sources.secrets_enc).
	FieldVariable FieldKind = "variable"
)

// Roles válidos de un campo en el DSN PostgreSQL. Conjunto CERRADO: un role no
// vacío fuera de este conjunto es inválido (se rechaza la plantilla). Un role
// ausente/vacío se trata como RoleOption con nombre = key.
const (
	RoleHost     = "host"
	RolePort     = "port"
	RoleUser     = "user"
	RolePassword = "password"
	RoleDBName   = "dbname"
	RoleSSLMode  = "sslmode"
	RoleOption   = "option"
)

// validRoles es el conjunto cerrado de roles aceptados (además del ausente/vacío).
var validRoles = map[string]struct{}{
	RoleHost:     {},
	RolePort:     {},
	RoleUser:     {},
	RolePassword: {},
	RoleDBName:   {},
	RoleSSLMode:  {},
	RoleOption:   {},
}

// IsValidRole indica si role pertenece al conjunto cerrado o está ausente/vacío
// (que se trata como RoleOption). Un string no vacío fuera del conjunto es
// inválido.
func IsValidRole(role string) bool {
	if role == "" {
		return true
	}
	_, ok := validRoles[role]
	return ok
}

// TemplateField es la ESTRUCTURA de un campo (sin su valor). Se serializa en
// connection_templates.fields (JSONB, NO sensible).
type TemplateField struct {
	Key       string    `json:"key"`
	Kind      FieldKind `json:"kind"`
	Role      string    `json:"role,omitempty"`      // host|port|user|password|dbname|sslmode|option
	Required  bool      `json:"required,omitempty"`  // solo variable; default true
	Suggested string    `json:"suggested,omitempty"` // solo variable; valor sugerido NO sensible
}

// Template es la vista PÚBLICA de una plantilla: nunca incluye los valores de
// los campos fijos (solo su estructura). HasFixedValues indica si hay bundle
// cifrado, sin revelarlo (igual que DataSource.HasSecrets).
type Template struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Fields         []TemplateField `json:"fields"`
	HasFixedValues bool            `json:"hasFixedValues"`
	UsedByModules  int             `json:"usedByModules"` // radio de impacto (D-4)
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// FixedValues es el bundle EN CLARO de valores FIJOS (clave->valor), solo para
// crear/editar y para el ensamblado en memoria. Nunca se serializa al frontend.
type FixedValues map[string]string

// Errores de negocio exportados del repositorio.
var (
	// ErrNotFound se devuelve cuando no existe la plantilla consultada.
	ErrNotFound = errors.New("connections: plantilla no encontrada")
	// ErrNameTaken se devuelve al crear/editar con un nombre ya existente.
	ErrNameTaken = errors.New("connections: el nombre de la plantilla ya está en uso")
	// ErrValidation se devuelve cuando la estructura de la plantilla es inválida
	// (sin campos, claves duplicadas, role inválido, campo FIJO sin valor).
	ErrValidation = errors.New("connections: la plantilla es inválida")
	// ErrTemplateInUse se devuelve al intentar borrar una plantilla referenciada
	// por al menos un módulo (D-5, Req. 2.6).
	ErrTemplateInUse = errors.New("connections: la plantilla está en uso por uno o más módulos")
)
