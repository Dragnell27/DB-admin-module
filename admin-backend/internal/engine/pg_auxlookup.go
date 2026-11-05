package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"admin-backend/internal/modules"
)

// resolveAuxLabels enriquece, de forma ADITIVA, las filas ya materializadas por
// scanRows con la etiqueta legible de cada columna que declara una fuente de
// valor DINÁMICA (ValueSource.Kind == "dynamic") apuntando a una tabla de la
// lista blanca (típicamente una tabla auxiliar/catálogo). Para cada columna así:
//
//  1. Recolecta los ids DISTINTOS presentes en la página, leyendo el valor crudo
//     bajo el alias de la fila (N=1: columna pelada "<col>"; multi:
//     "<tabla>__<col>").
//  2. Ejecuta UN solo SELECT "<idCol>","<labelCol>" FROM "<tabla>" WHERE
//     "<idCol>" IN ($1..$k) [AND filtros del catálogo] por catálogo por página
//     (opción b: lote, sin JOIN, sin N+1). La tabla y las columnas se validan con
//     checkTable/checkColumnWhitelisted y cada id se liga como $N (nunca se
//     interpola). Reutiliza normalizeSQLValue para que []byte serialice como texto.
//  3. Escribe la etiqueta resuelta en una CLAVE SEPARADA de la fila sin pisar el
//     id crudo: N=1 "<col>__label"; multi "<tabla>__<col>__label".
//
// Degradación segura: si el pool falla, un SELECT falla o un id no resuelve, la
// columna muestra el id crudo sin romper el listado (no se añade "__label" para
// los ids no resueltos). Si no hay columnas dinámicas a resolver, 0 consultas
// extra y las filas quedan byte-por-byte como hoy.
func (e *PGExecutor) resolveAuxLabels(ctx context.Context, def modules.Definition, spec modules.MultiTableSpec, rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, spec.PrincipalTable)
	multi := spec.IsMultiTable()

	// Columnas con fuente dinámica a resolver, en orden estable (Field, SourceTable).
	type dynCol struct {
		rowKey   string // alias bajo el que viaja el id crudo en la fila
		labelKey string // clave separada donde se escribe la etiqueta
		src      *modules.DynamicSource
	}
	dyn := make([]dynCol, 0)
	for _, c := range cols {
		vs := c.ValueSource
		if vs == nil || vs.Kind != "dynamic" || vs.DynamicSource == nil {
			continue
		}
		rowKey := c.Field
		labelKey := c.Field + "__label"
		if multi {
			rowKey = c.SourceTable + "__" + c.Field
			labelKey = c.SourceTable + "__" + c.Field + "__label"
		}
		dyn = append(dyn, dynCol{rowKey: rowKey, labelKey: labelKey, src: vs.DynamicSource})
	}
	if len(dyn) == 0 {
		return
	}

	db, err := e.pool(ctx, def)
	if err != nil {
		return // degradación: se muestran los ids crudos sin romper el listado
	}
	whitelist := def.DataSource.PGWhitelist

	for _, dc := range dyn {
		labels, err := auxLabelsForColumn(ctx, db, whitelist, dc.src, rows, dc.rowKey)
		if err != nil {
			continue // degradación por columna: id crudo
		}
		if len(labels) == 0 {
			continue
		}
		for _, row := range rows {
			raw, ok := row[dc.rowKey]
			if !ok || raw == nil {
				continue
			}
			if label, ok := labels[auxIDKey(raw)]; ok {
				row[dc.labelKey] = label
			}
		}
	}
}

// auxLabelsForColumn ejecuta el lookup en LOTE de una columna dinámica: compone y
// valida "SELECT <idCol>,<labelCol> FROM <tabla> WHERE <idCol> IN ($1..$k)
// [AND filtros] LIMIT <DefaultOptionsLimit>", ligando cada id DISTINTO de la
// página como $N y numerando los filtros del catálogo después. Valida la tabla y
// las columnas id/etiqueta/filtro contra la lista blanca ANTES de ejecutar;
// cualquier identificador fuera de lista aborta con ErrWhitelist (el llamador
// degrada al id). Devuelve map[idTexto]etiqueta; ids sin fila quedan ausentes.
func auxLabelsForColumn(ctx context.Context, db *sql.DB, whitelist map[string][]string, src *modules.DynamicSource, rows []map[string]any, rowKey string) (map[string]any, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: la columna dinámica no declara catálogo", ErrValidation)
	}
	if strings.TrimSpace(src.Table) == "" ||
		strings.TrimSpace(src.IDColumn) == "" ||
		strings.TrimSpace(src.LabelColumn) == "" {
		return nil, fmt.Errorf("%w: el origen dinámico requiere tabla, columna id y columna etiqueta", ErrValidation)
	}
	// Tabla y columnas id/etiqueta en la lista blanca.
	if err := checkTable(whitelist, src.Table); err != nil {
		return nil, err
	}
	if err := checkColumnWhitelisted(whitelist, src.Table, src.IDColumn); err != nil {
		return nil, err
	}
	if err := checkColumnWhitelisted(whitelist, src.Table, src.LabelColumn); err != nil {
		return nil, err
	}

	// Ids DISTINTOS de la página, en orden de aparición (orden de $N estable).
	ids := make([]any, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		raw, ok := row[rowKey]
		if !ok || raw == nil {
			continue
		}
		k := auxIDKey(raw)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		ids = append(ids, raw)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Marcadores IN ($1..$k) para los ids; los filtros del catálogo se numeran
	// DESPUÉS ($k+1..). Cada valor viaja ligado; nunca se interpola.
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, ids[i])
	}

	condClause, condArgs, err := buildListConditionsWhere(whitelist, src.Table, src.Filters, len(ids)+1)
	if err != nil {
		return nil, err
	}

	whereSQL := fmt.Sprintf("%s IN (%s)", quoteIdent(src.IDColumn), strings.Join(placeholders, ", "))
	if condClause != "" {
		whereSQL += " AND " + condClause
	}
	args = append(args, condArgs...)

	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s LIMIT %d",
		joinQuotedIdents([]string{src.IDColumn, src.LabelColumn}),
		quoteIdent(src.Table), whereSQL, DefaultOptionsLimit,
	)

	dbRows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("engine: error ejecutando la relectura de etiquetas: %w", err)
	}
	defer dbRows.Close()

	out := make(map[string]any, len(ids))
	for dbRows.Next() {
		var id, label any
		if err := dbRows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("engine: error leyendo etiqueta auxiliar: %w", err)
		}
		out[auxIDKey(normalizeSQLValue(id))] = normalizeSQLValue(label)
	}
	if err := dbRows.Err(); err != nil {
		return nil, fmt.Errorf("engine: error iterando etiquetas auxiliares: %w", err)
	}
	return out, nil
}

// auxIDKey normaliza un id (de la fila o de la relectura del catálogo) a una
// clave de texto estable para emparejar id↔etiqueta, independientemente de que
// el driver lo entregue como int64, float64, []byte o string. Reutiliza
// normalizeSQLValue para que []byte y string coincidan.
func auxIDKey(v any) string {
	return fmt.Sprintf("%v", normalizeSQLValue(v))
}
