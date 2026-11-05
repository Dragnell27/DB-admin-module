package modules

import (
	"encoding/json"
	"errors"
)

// ErrMultiTableValidation es el sentinel compartido que TODOS los rechazos de
// validateMultiTable envuelven con %w (fmt.Errorf("%w: mensaje…", ...)). El
// handler de guardado (writeDefError) lo mapea con errors.Is a E422_VALIDATION,
// conservando el mensaje español específico; sin este sentinel caería al E500
// genérico.
var ErrMultiTableValidation = errors.New("validación multi-tabla")

// Tipos de JOIN soportados en v1 (lectura). RIGHT/FULL quedan fuera de alcance.
const (
	JoinInner = "inner"
	JoinLeft  = "left"
)

// ForeignTable declara EN LÍNEA una tabla foránea del módulo: solo su nombre.
// Sus columnas, PK, visibilidad y editabilidad viven en displayMapping.columns
// con sourceTable == Table; su lista blanca vive en DataSource.PGWhitelist[Table].
// No necesita un módulo propio.
type ForeignTable struct {
	Table string `json:"table"`
}

// JoinPair es un par de columnas del ON dirigido por FK:
// childTable.Child = parentTable.Parent.
type JoinPair struct {
	Child  string `json:"child"`  // columna FK en ChildTable
	Parent string `json:"parent"` // columna clave en ParentTable
}

// JoinConstant es una condición extra del ON que compara una columna de UNA de
// las dos tablas de la relación contra un valor CONSTANTE. El valor viaja SIEMPRE
// como parámetro ligado ($N); NUNCA se interpola. Table debe ser ChildTable o
// ParentTable de la relación.
type JoinConstant struct {
	Table  string `json:"table"`  // ChildTable o ParentTable de la relación
	Column string `json:"column"` // columna en Table (whitelisteada)
	Value  string `json:"value"`  // literal; se liga como $N (Postgres coacciona el tipo)
}

// Relationship declara UN JOIN manual DIRIGIDO entre dos tablas. Por convención
// FIJA de v1, ChildTable es la tabla que POSEE la FK (referencia) y ParentTable
// es la tabla referenciada. El orden de borrado hijo→padre se deriva EXCLUSIVA
// mente de esta dirección (ChildTable se borra antes que ParentTable), NUNCA de
// la distancia a la principal. Para la LECTURA (SELECT con JOIN) la dirección
// solo compone el ON.
type Relationship struct {
	// ChildTable posee la FK (referencia). Se borra ANTES que ParentTable.
	ChildTable string `json:"childTable"`
	// ParentTable es la tabla referenciada (la clave a la que apunta la FK).
	ParentTable string `json:"parentTable"`
	// On empareja columnas: On[i].Child (FK en ChildTable) = On[i].Parent (clave
	// en ParentTable).
	On []JoinPair `json:"on"`
	// OnConstants son condiciones extra "<tabla>.<col> = $N" del ON, ANDed con los
	// pares On[]. Opcional y omitempty: una definición sin él se comporta como hoy.
	OnConstants []JoinConstant `json:"onConstants,omitempty"`
	// JoinType es "inner"|"left" en v1.
	JoinType string `json:"joinType"`
}

// AuxTable declara una TABLA AUXILIAR (catálogo) del módulo: una tabla de la
// misma conexión cuyas columnas sirven SOLO para resolver id→etiqueta en el
// listado (vía una columna con valueSource.dynamic), NUNCA como columna del grid
// ni como tabla del árbol de relaciones. A diferencia de una ForeignTable, NO
// entra en Tables() (no exige JOIN ni grafo conexo); su único efecto es fundir
// Columns en la lista blanca efectiva para que el motor y el endpoint de options
// las acepten. Table es el nombre de la tabla; Columns la whitelist de columnas
// que el catálogo puede leer (id/etiqueta/filtros).
type AuxTable struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
}

// MultiTableSpec es la vista tipada de fetchMapping para multi-tabla.
type MultiTableSpec struct {
	PrincipalTable string
	ForeignTables  []ForeignTable
	Relationships  []Relationship
	// AuxTables son las tablas auxiliares (catálogos) del módulo. NO forman parte
	// de Tables() ni de IsMultiTable(): son catálogos para resolver etiquetas, no
	// tablas del árbol.
	AuxTables []AuxTable
}

// fetchMappingMultiTable es la forma JSON intermedia para decodificar las claves
// nuevas de fetchMapping sin tocar el resto del blob opaco.
type fetchMappingMultiTable struct {
	ForeignTables []ForeignTable `json:"foreignTables"`
	Relationships []Relationship `json:"relationships"`
	AuxTables     []AuxTable     `json:"auxTables"`
}

// MultiTableFromFetchMapping decodifica la vista multi-tabla de fetchMapping con
// un round-trip JSON tolerante (mismo patrón que decodeConditions): nunca panica
// y ante JSONB malformado devuelve una spec N=1 (solo principal). PrincipalTable
// sale de fm["table"] (igual que hoy). foreignTables/relationships ausentes ⇒
// slices vacíos ⇒ N=1.
func MultiTableFromFetchMapping(fm map[string]any) MultiTableSpec {
	spec := MultiTableSpec{}
	if fm == nil {
		return spec
	}
	if t, ok := fm["table"].(string); ok {
		spec.PrincipalTable = t
	}
	b, err := json.Marshal(fm)
	if err != nil {
		return spec
	}
	var raw fetchMappingMultiTable
	if err := json.Unmarshal(b, &raw); err != nil {
		return spec
	}
	spec.ForeignTables = raw.ForeignTables
	spec.Relationships = raw.Relationships
	spec.AuxTables = raw.AuxTables
	return spec
}

// AuxWhitelist devuelve la lista blanca aportada por las tablas auxiliares:
// map[table][]columns, sin duplicar columnas por tabla y fusionando varias
// entradas que declaren la misma tabla. Tablas con nombre vacío se omiten. Es la
// base para fundir los catálogos en la whitelist efectiva (repository.go) sin
// que entren a Tables().
func (s MultiTableSpec) AuxWhitelist() map[string][]string {
	if len(s.AuxTables) == 0 {
		return nil
	}
	out := make(map[string][]string, len(s.AuxTables))
	for _, aux := range s.AuxTables {
		if aux.Table == "" {
			continue
		}
		seen := make(map[string]struct{}, len(out[aux.Table]))
		for _, c := range out[aux.Table] {
			seen[c] = struct{}{}
		}
		for _, c := range aux.Columns {
			if c == "" {
				continue
			}
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			out[aux.Table] = append(out[aux.Table], c)
		}
	}
	return out
}

// IsMultiTable es el ÚNICO predicado que decide si un módulo es multi-tabla. Lo
// consumen EditFieldSpecs, List, Exec, module-view (vía JSON) y el frontend, de
// modo que ninguna capa discrepe sobre alias vs nombre pelado.
func (s MultiTableSpec) IsMultiTable() bool {
	return len(s.ForeignTables) > 0 || len(s.Relationships) > 0
}

// Tables devuelve todas las tablas declaradas del módulo (principal + foráneas),
// sin duplicados, con la principal primero y las foráneas en orden de
// declaración.
func (s MultiTableSpec) Tables() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(s.ForeignTables)+1)
	if s.PrincipalTable != "" {
		out = append(out, s.PrincipalTable)
		seen[s.PrincipalTable] = struct{}{}
	}
	for _, ft := range s.ForeignTables {
		if ft.Table == "" {
			continue
		}
		if _, ok := seen[ft.Table]; ok {
			continue
		}
		seen[ft.Table] = struct{}{}
		out = append(out, ft.Table)
	}
	return out
}

// normalizeJoinType devuelve el tipo de JOIN normalizado: "left" solo si el
// literal es exactamente "left"; cualquier otro valor (vacío o desconocido) cae
// a "inner". Es el ÚNICO punto que decide el literal; nunca se interpola el
// valor crudo en el SQL.
func normalizeJoinType(jt string) string {
	if jt == JoinLeft {
		return JoinLeft
	}
	return JoinInner
}
