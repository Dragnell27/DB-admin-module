package engine

import (
	"context"
	"fmt"
	"strings"

	"admin-backend/internal/modules"
)

// DefaultOptionsLimit es el tope de filas que devuelve la consulta de opciones
// de un origen dinámico. Acota el tamaño del desplegable y protege al origen de
// una tabla de valores muy grande. También es el valor por defecto cuando el
// origen no declara (o declara un límite inválido).
const DefaultOptionsLimit = 500

// Option es una opción (etiqueta/valor) de un campo con Source=="dynamic": ID es
// la clave foránea que se guarda al elegirla; Label es el texto visible. Se
// serializan tal cual en la respuesta del endpoint de opciones.
type Option struct {
	ID    any `json:"id"`
	Label any `json:"label"`
}

// normalizeDirection normaliza la dirección de ordenamiento a un literal seguro:
// "desc" (sin distinguir mayúsculas) ⇒ "desc"; cualquier otra cosa ⇒ "asc". Así
// el valor emitido en el ORDER BY nunca procede directamente de la entrada.
func normalizeDirection(dir string) string {
	if strings.EqualFold(strings.TrimSpace(dir), "desc") {
		return "desc"
	}
	return "asc"
}

// effectiveLimit resuelve el LIMIT efectivo: un valor no positivo o mayor que
// DefaultOptionsLimit cae al tope por defecto; en otro caso se respeta tal cual.
func effectiveLimit(n int) int {
	if n <= 0 || n > DefaultOptionsLimit {
		return DefaultOptionsLimit
	}
	return n
}

// buildOptionsSQL compone el SQL parametrizado que obtiene las opciones
// (id/etiqueta) de un origen dinámico, validando TODO identificador contra la
// lista blanca del módulo ANTES de producir texto. Reutiliza la infraestructura
// de seguridad existente: checkTable, buildListConditionsWhere (filtros ⇒ WHERE
// parametrizado), quoteIdent/joinQuotedIdents y ErrWhitelist. El SQL resultante
// es EXACTAMENTE:
//
//	SELECT <idCol>, <labelCol> FROM <tabla> [WHERE ...] [ORDER BY <col> asc|desc] LIMIT $N
//
// Los valores de filtro viajan SIEMPRE como parámetros ligados ($1..$k) y el
// LIMIT se numera DESPUÉS de los args del WHERE ($k+1). Devuelve un error que
// envuelve ErrValidation si falta tabla/id/etiqueta, o ErrWhitelist si algún
// identificador (tabla, id, etiqueta, columna de filtro u orden) no está en la
// lista blanca; en ambos casos SIN producir SQL.
func buildOptionsSQL(whitelist map[string][]string, src modules.DynamicSource) (string, []any, error) {
	// Completitud: el origen debe declarar tabla, columna id y columna etiqueta.
	if strings.TrimSpace(src.Table) == "" ||
		strings.TrimSpace(src.IDColumn) == "" ||
		strings.TrimSpace(src.LabelColumn) == "" {
		return "", nil, fmt.Errorf("%w: el origen dinámico requiere tabla, columna id y columna etiqueta", ErrValidation)
	}

	// Tabla en la lista blanca.
	if err := checkTable(whitelist, src.Table); err != nil {
		return "", nil, err
	}

	// Columnas id y etiqueta en la lista blanca de la tabla.
	allowedSet := make(map[string]struct{}, len(whitelist[src.Table]))
	for _, c := range whitelist[src.Table] {
		allowedSet[c] = struct{}{}
	}
	if _, ok := allowedSet[src.IDColumn]; !ok {
		return "", nil, fmt.Errorf("%w: columna id %q en la tabla %q", ErrWhitelist, src.IDColumn, src.Table)
	}
	if _, ok := allowedSet[src.LabelColumn]; !ok {
		return "", nil, fmt.Errorf("%w: columna etiqueta %q en la tabla %q", ErrWhitelist, src.LabelColumn, src.Table)
	}

	// WHERE parametrizado a partir de los filtros, numerado desde $1 (valida cada
	// columna de filtro contra la lista blanca ⇒ ErrWhitelist sin SQL).
	condClause, condArgs, err := buildListConditionsWhere(whitelist, src.Table, src.Filters, 1)
	if err != nil {
		return "", nil, err
	}
	whereSQL := ""
	if condClause != "" {
		whereSQL = " WHERE " + condClause
	}

	// ORDER BY opcional: solo si se declara columna. Se valida contra la lista
	// blanca (ErrWhitelist) y la dirección se normaliza a asc|desc.
	orderSQL := ""
	if src.Order != nil && strings.TrimSpace(src.Order.Column) != "" {
		col := src.Order.Column
		if _, ok := allowedSet[col]; !ok {
			return "", nil, fmt.Errorf("%w: columna de orden %q en la tabla %q", ErrWhitelist, col, src.Table)
		}
		orderSQL = " ORDER BY " + quoteIdent(col) + " " + normalizeDirection(src.Order.Direction)
	}

	// LIMIT resuelto (tope por defecto) numerado DESPUÉS de los args del WHERE.
	limit := effectiveLimit(src.Limit)
	sql := fmt.Sprintf(
		"SELECT %s FROM %s%s%s LIMIT $%d",
		joinQuotedIdents([]string{src.IDColumn, src.LabelColumn}),
		quoteIdent(src.Table), whereSQL, orderSQL, len(condArgs)+1,
	)
	args := append(condArgs, limit)
	return sql, args, nil
}

// Options ejecuta la consulta de opciones de un origen dinámico y devuelve la
// lista de pares id/etiqueta. Construye y VALIDA el SQL con buildOptionsSQL
// ANTES de tocar la BD: si la validación falla (ErrValidation/ErrWhitelist),
// retorna el error sin abrir el pool. En caso válido abre el pool del módulo y
// ejecuta la consulta, escaneando dos columnas por fila (id, etiqueta) con
// normalizeSQLValue (igual que scanRows) para que []byte serialice como texto.
//
// NO se añade a la interfaz Executor (engine.go): queda en el tipo concreto
// *PGExecutor porque solo el camino Postgres soporta orígenes dinámicos.
func (e *PGExecutor) Options(ctx context.Context, def modules.Definition, src modules.DynamicSource) ([]Option, error) {
	sql, args, err := buildOptionsSQL(def.DataSource.PGWhitelist, src)
	if err != nil {
		return nil, err
	}

	db, err := e.pool(ctx, def)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("engine: error ejecutando la consulta de opciones: %w", err)
	}
	defer rows.Close()

	out := []Option{}
	for rows.Next() {
		var id, label any
		if err := rows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("engine: error leyendo opción: %w", err)
		}
		out = append(out, Option{ID: normalizeSQLValue(id), Label: normalizeSQLValue(label)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engine: error iterando opciones: %w", err)
	}
	return out, nil
}
