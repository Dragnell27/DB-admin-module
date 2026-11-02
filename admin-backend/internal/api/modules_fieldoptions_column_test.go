package api

import (
	"testing"

	"admin-backend/internal/modules"
)

// NOTA (límite del harness): FieldOptions depende de h.repo.FindBySlug y de
// pg.Options(*engine.PGExecutor), ambos acoplados a una base de datos Postgres
// real; el paquete internal/api no tiene un harness que los simule sin BD. Por
// eso, siguiendo lo previsto en FEAT-002, se degrada la cobertura a un test de
// FUNCIÓN PURA de resolveColumnDynamicSource, que concentra la lógica nueva:
// emparejamiento por (field, sourceTable), principal por ausencia de sourceTable
// (desambiguación de homónimas) y filtrado de columnas estáticas o sin fuente.

// displayMappingConValueSource construye un displayMapping con columnas que
// declaran (o no) valueSource, para alimentar resolveColumnDynamicSource a
// través de ColumnsFromDisplayMappingWithPrincipal (round-trip JSON tolerante).
func defConColumnas(principal string, cols []modules.DisplayColumn) *modules.Definition {
	rawCols := make([]map[string]any, 0, len(cols))
	for _, c := range cols {
		m := map[string]any{"field": c.Field}
		if c.SourceTable != "" {
			m["sourceTable"] = c.SourceTable
		}
		if c.ValueSource != nil {
			vs := map[string]any{"kind": c.ValueSource.Kind}
			if len(c.ValueSource.Options) > 0 {
				vs["options"] = c.ValueSource.Options
			}
			if c.ValueSource.DynamicSource != nil {
				vs["dynamicSource"] = map[string]any{
					"table":       c.ValueSource.DynamicSource.Table,
					"idColumn":    c.ValueSource.DynamicSource.IDColumn,
					"labelColumn": c.ValueSource.DynamicSource.LabelColumn,
				}
			}
			m["valueSource"] = vs
		}
		rawCols = append(rawCols, m)
	}
	fetch := map[string]any{}
	if principal != "" {
		fetch["table"] = principal
	}
	return &modules.Definition{
		FetchMapping:   fetch,
		DisplayMapping: map[string]any{"columns": rawCols},
	}
}

func dynSrc(table, id, label string) *modules.ColumnValueSource {
	return &modules.ColumnValueSource{
		Kind: "dynamic",
		DynamicSource: &modules.DynamicSource{
			Table:       table,
			IDColumn:    id,
			LabelColumn: label,
		},
	}
}

func staticSrc() *modules.ColumnValueSource {
	return &modules.ColumnValueSource{
		Kind:    "static",
		Options: []modules.SelectOption{{Label: "Activo", Value: "1"}},
	}
}

// Una columna con valueSource.dynamic resuelve su DynamicSource.
func TestResolveColumnDynamicSource_Dinamica(t *testing.T) {
	def := defConColumnas("principal", []modules.DisplayColumn{
		{Field: "estado_id", ValueSource: dynSrc("estados", "id", "nombre")},
	})
	src, ok := resolveColumnDynamicSource(def, "estado_id", "", "principal")
	if !ok {
		t.Fatalf("se esperaba ok=true para columna dinámica")
	}
	if src.Table != "estados" || src.IDColumn != "id" || src.LabelColumn != "nombre" {
		t.Fatalf("DynamicSource resuelto inesperado: %+v", src)
	}
}

// Una columna estática (sin DynamicSource) NO resuelve: ok=false (=> 404).
func TestResolveColumnDynamicSource_EstaticaDaFalse(t *testing.T) {
	def := defConColumnas("principal", []modules.DisplayColumn{
		{Field: "estado_id", ValueSource: staticSrc()},
	})
	if _, ok := resolveColumnDynamicSource(def, "estado_id", "", "principal"); ok {
		t.Fatalf("una columna estática no debe resolver fuente dinámica")
	}
}

// Una columna sin valueSource NO resuelve: ok=false (=> 404).
func TestResolveColumnDynamicSource_SinFuenteDaFalse(t *testing.T) {
	def := defConColumnas("principal", []modules.DisplayColumn{
		{Field: "estado_id"},
	})
	if _, ok := resolveColumnDynamicSource(def, "estado_id", "", "principal"); ok {
		t.Fatalf("una columna sin valueSource no debe resolver fuente dinámica")
	}
}

// Una columna inexistente NO resuelve: ok=false (=> 404).
func TestResolveColumnDynamicSource_InexistenteDaFalse(t *testing.T) {
	def := defConColumnas("principal", []modules.DisplayColumn{
		{Field: "estado_id", ValueSource: dynSrc("estados", "id", "nombre")},
	})
	if _, ok := resolveColumnDynamicSource(def, "otro_campo", "", "principal"); ok {
		t.Fatalf("una columna inexistente no debe resolver fuente dinámica")
	}
}

// Multi-tabla: &sourceTable= desambigua columnas homónimas; sin sourceTable se
// toma la de la tabla principal.
func TestResolveColumnDynamicSource_MultiTablaDesambigua(t *testing.T) {
	def := defConColumnas("principal", []modules.DisplayColumn{
		{Field: "estado", SourceTable: "principal", ValueSource: dynSrc("estados_p", "id", "nombre")},
		{Field: "estado", SourceTable: "secundaria", ValueSource: dynSrc("estados_s", "id", "nombre")},
	})

	// Sin sourceTable => principal.
	src, ok := resolveColumnDynamicSource(def, "estado", "", "principal")
	if !ok || src.Table != "estados_p" {
		t.Fatalf("sin sourceTable debe tomar la principal; ok=%v src=%+v", ok, src)
	}

	// Con sourceTable explícito => la homónima correcta.
	src, ok = resolveColumnDynamicSource(def, "estado", "secundaria", "principal")
	if !ok || src.Table != "estados_s" {
		t.Fatalf("con sourceTable debe tomar la homónima; ok=%v src=%+v", ok, src)
	}
}
