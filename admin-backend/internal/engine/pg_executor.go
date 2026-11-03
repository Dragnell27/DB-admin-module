package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"admin-backend/internal/modules"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrWhitelist indica que una operación Postgres referencia una tabla o columna
// fuera de la lista blanca de la Definición de módulo. El router lo traduce al
// errorCode E422_PG_WHITELIST sin ejecutar (Req. 4.5, 4.6).
var ErrWhitelist = errors.New("engine: identificador fuera de la lista blanca")

// sqlOpener abstrae la apertura de un pool *sql.DB para inyectar un doble en
// pruebas y evitar conexiones reales.
type sqlOpener func(dsn string) (*sql.DB, error)

// PGExecutor ejecuta módulos cuyo origen es PostgreSQL. TODA consulta usa
// parámetros ligados ($1,$2,...) y valida identificadores (tablas/columnas)
// contra la lista blanca del módulo ANTES de componer el SQL (Req. 4.4, 4.5,
// 4.6). Mantiene un pool por módulo (clave: slug).
type PGExecutor struct {
	secrets SecretProvider
	open    sqlOpener

	mu    sync.Mutex
	pools map[string]*sql.DB
}

// NewPGExecutor construye un PGExecutor con el proveedor de secretos (para
// descifrar el DSN) y un abridor de pool. Si opener es nil usa sql.Open con el
// driver pgx estándar del proyecto.
func NewPGExecutor(sp SecretProvider, opener sqlOpener) *PGExecutor {
	if opener == nil {
		opener = func(dsn string) (*sql.DB, error) { return sql.Open("pgx", dsn) }
	}
	return &PGExecutor{secrets: sp, open: opener, pools: make(map[string]*sql.DB)}
}

// pool devuelve (abriendo si hace falta) el pool del módulo, descifrando su DSN.
func (e *PGExecutor) pool(ctx context.Context, def modules.Definition) (*sql.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if db, ok := e.pools[def.Slug]; ok {
		return db, nil
	}
	sec, err := e.secrets.RevealSecrets(ctx, def.Slug)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sec.DSN) == "" {
		return nil, fmt.Errorf("engine: el módulo Postgres %q no tiene cadena de conexión", def.Slug)
	}
	db, err := e.open(sec.DSN)
	if err != nil {
		return nil, fmt.Errorf("engine: no se pudo abrir la conexión Postgres del módulo %q: %w", def.Slug, err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	e.pools[def.Slug] = db
	return db, nil
}

// Invalidate cierra y descarta el pool cacheado del módulo `slug` para que la
// próxima ejecución lo reabra con el DSN efectivo recalculado (D-4, Req. 3.5).
// Es un no-op si no hay pool abierto para ese slug. *PGExecutor satisface así la
// interfaz PoolInvalidator además de Executor.
func (e *PGExecutor) Invalidate(slug string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if db, ok := e.pools[slug]; ok {
		_ = db.Close()
		delete(e.pools, slug)
	}
}

// List ejecuta la Acción Listar (un SELECT) sobre la tabla declarada, limitando
// las columnas a la lista blanca y paginando con LIMIT/OFFSET parametrizados.
// Es el listado BASE (sin filtro de acción): delega en listCore con la acción
// cero valor e input nil, de modo que el SQL es byte-por-byte el de hoy.
func (e *PGExecutor) List(ctx context.Context, def modules.Definition, country string, q ListQuery) ([]map[string]any, Pagination, error) {
	return e.listCore(ctx, def, country, q, modules.Action{}, nil)
}

// ListFiltered ejecuta el listado del módulo aplicando, además de las
// condiciones estáticas, el WHERE derivado de los campos LLENOS de la acción
// SELECT. Comparte exactamente el cuerpo de List (pool, COUNT/SELECT,
// LIMIT/OFFSET, scanRows) vía listCore, pasando act,input a buildListSQL.
func (e *PGExecutor) ListFiltered(ctx context.Context, def modules.Definition, country string, q ListQuery, act modules.Action, input map[string]any) ([]map[string]any, Pagination, error) {
	return e.listCore(ctx, def, country, q, act, input)
}

// listCore es el cuerpo compartido por List y ListFiltered: compone el SELECT +
// COUNT (opcionalmente con el WHERE de filtro de la acción), abre el pool y
// pagina. Con act cero valor e input nil el SQL es idéntico al listado base.
func (e *PGExecutor) listCore(ctx context.Context, def modules.Definition, country string, q ListQuery, act modules.Action, input map[string]any) ([]map[string]any, Pagination, error) {
	listAct, ok := findListAction(def)
	if !ok {
		return nil, Pagination{}, fmt.Errorf("engine: el módulo %q no define una acción Listar", def.Slug)
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	_ = listAct

	// Composición del SELECT + COUNT. En N=1 se emite EXACTAMENTE el SQL de hoy
	// (columnas peladas, FROM sin JOIN); en multi-tabla, columnas aliased
	// <tabla>__<col> con los JOIN declarados. Toda la validación de lista blanca
	// e identificadores ocurre ANTES de abrir el pool o ejecutar SQL.
	selectSQLBody, outputNames, condArgs, err := buildListSQL(def, spec, act, input)
	if err != nil {
		return nil, Pagination{}, err
	}

	db, err := e.pool(ctx, def)
	if err != nil {
		return nil, Pagination{}, err
	}

	page := q.Page
	if page <= 0 {
		page = 1
	}
	per := q.RecordsPerPage
	if per <= 0 {
		per = 25
	}
	offset := (page - 1) * per

	// Cuenta total: comparte el MISMO FROM/JOIN/WHERE y los MISMOS args que el
	// SELECT de datos (paginación consistente, R4.6).
	var total int64
	countSQL := "SELECT count(*) " + selectSQLBody.fromWhere
	if err := db.QueryRowContext(ctx, countSQL, condArgs...).Scan(&total); err != nil {
		return nil, Pagination{}, fmt.Errorf("engine: error contando filas: %w", err)
	}

	// LIMIT/OFFSET se numeran DESPUÉS de los parámetros del WHERE.
	selectSQL := fmt.Sprintf(
		"SELECT %s %s LIMIT $%d OFFSET $%d",
		selectSQLBody.projection, selectSQLBody.fromWhere, len(condArgs)+1, len(condArgs)+2,
	)
	selectArgs := append(append([]any{}, condArgs...), per, offset)
	rows, err := db.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("engine: error ejecutando SELECT: %w", err)
	}
	defer rows.Close()

	result, err := scanRows(rows, outputNames)
	if err != nil {
		return nil, Pagination{}, err
	}
	totalPages := int((total + int64(per) - 1) / int64(per))
	return result, Pagination{TotalRecords: total, Page: page, RecordsPerPage: per, TotalPages: totalPages}, nil
}

// listSQLParts contiene las partes reutilizables del SELECT de listado: la
// proyección de columnas (lo que va tras SELECT) y el bloque FROM [JOIN] [WHERE]
// (compartido por el COUNT y el SELECT de datos).
type listSQLParts struct {
	projection string
	fromWhere  string
}

// buildListSQL compone, sin ejecutar, las partes del SELECT de listado y la
// lista de nombres de salida que recibirá scanRows. Despacha N=1 vs multi-tabla
// por spec.IsMultiTable(). Valida identificadores contra la lista blanca ANTES
// de componer; cualquier columna/tabla fuera de lista aborta con ErrWhitelist.
//
// La acción `act` y el mapa `input` aportan el WHERE de filtro de una acción
// SELECT: con `act` cero valor (modules.Action{}) e `input` nil,
// buildActionFilterWhere devuelve ("", nil, nil) y el SQL queda BYTE POR BYTE
// idéntico al listado base (GET /list).
func buildListSQL(def modules.Definition, spec modules.MultiTableSpec, act modules.Action, input map[string]any) (listSQLParts, []string, []any, error) {
	whitelist := def.DataSource.PGWhitelist

	if !spec.IsMultiTable() {
		// --- Camino N=1 byte-por-byte como hoy (sin filtros) ---
		table := stringFromConfig(def.FetchMapping, "table", "")
		if err := checkTable(whitelist, table); err != nil {
			return listSQLParts{}, nil, nil, err
		}
		// Columnas resueltas (principal vacío ⇒ SourceTable==""): alimentan la
		// proyección y el builder de filtro de acción.
		resolved := modules.ColumnsFromDisplayMapping(def.DisplayMapping)
		cols, err := selectColumns(whitelist, table, def.ListConditions)
		if err != nil {
			return listSQLParts{}, nil, nil, err
		}
		if len(cols) == 0 {
			return listSQLParts{}, nil, nil, fmt.Errorf("%w: la tabla %q no declara columnas", ErrWhitelist, table)
		}
		// WHERE estático (condiciones de listado) numerado desde $1.
		condClause, staticArgs, err := buildListConditionsWhere(whitelist, table, def.ListConditions, 1)
		if err != nil {
			return listSQLParts{}, nil, nil, err
		}
		// WHERE del filtro de acción SELECT: continúa la numeración de los estáticos.
		filterClause, filterArgs, err := buildActionFilterWhere(whitelist, resolved, spec, act, input, len(staticArgs)+1)
		if err != nil {
			return listSQLParts{}, nil, nil, err
		}
		merged := mergeWhereClauses(condClause, filterClause)
		condArgs := append(append([]any{}, staticArgs...), filterArgs...)
		whereSQL := ""
		if merged != "" {
			whereSQL = " WHERE " + merged
		}
		parts := listSQLParts{
			projection: joinQuotedIdents(cols),
			fromWhere:  fmt.Sprintf("FROM %s%s", quoteIdent(table), whereSQL),
		}
		return parts, cols, condArgs, nil
	}

	// --- Camino multi-tabla ---
	principal := spec.PrincipalTable
	if err := checkTable(whitelist, principal); err != nil {
		return listSQLParts{}, nil, nil, err
	}
	// Cada tabla declarada debe tener entrada en la lista blanca.
	for _, t := range spec.Tables() {
		if err := checkTable(whitelist, t); err != nil {
			return listSQLParts{}, nil, nil, err
		}
	}

	cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, principal)
	selCols, err := selectColumnsMulti(whitelist, cols, spec, def.ListConditions)
	if err != nil {
		return listSQLParts{}, nil, nil, err
	}
	if len(selCols) == 0 {
		return listSQLParts{}, nil, nil, fmt.Errorf("%w: el módulo %q no declara columnas", ErrWhitelist, principal)
	}

	// Proyección: "<tabla>"."<col>" AS "<tabla>__<col>" en orden estable.
	projParts := make([]string, len(selCols))
	outputNames := make([]string, len(selCols))
	for i, qc := range selCols {
		alias := qc.alias()
		projParts[i] = fmt.Sprintf("%s.%s AS %s", quoteIdent(qc.table), quoteIdent(qc.column), quoteIdent(alias))
		outputNames[i] = alias
	}

	// FROM principal + JOINs recorriendo las relaciones en orden estable. Las
	// constantes del ON ligan sus valores como $N a partir de $1, ANTES del WHERE.
	fromClause, joinArgs, err := buildJoinClause(whitelist, spec, 1)
	if err != nil {
		return listSQLParts{}, nil, nil, err
	}

	// WHERE estático (condiciones de listado): continúa la numeración tras los
	// parámetros del ON para que los $N no colisionen.
	condClause, staticArgs, err := buildListConditionsWhereMulti(whitelist, cols, spec, def.ListConditions, len(joinArgs)+1)
	if err != nil {
		return listSQLParts{}, nil, nil, err
	}
	// WHERE del filtro de acción SELECT: continúa la numeración de ON + estáticos.
	filterClause, filterArgs, err := buildActionFilterWhere(whitelist, cols, spec, act, input, len(joinArgs)+len(staticArgs)+1)
	if err != nil {
		return listSQLParts{}, nil, nil, err
	}
	merged := mergeWhereClauses(condClause, filterClause)
	// Orden final de args (= orden de $N en el SQL): JOIN ON → WHERE estático →
	// WHERE filtro → LIMIT/OFFSET. COUNT y SELECT comparten estos mismos condArgs.
	condArgs := append(append(append([]any{}, joinArgs...), staticArgs...), filterArgs...)
	whereSQL := ""
	if merged != "" {
		whereSQL = " WHERE " + merged
	}

	parts := listSQLParts{
		projection: strings.Join(projParts, ", "),
		fromWhere:  fromClause + whereSQL,
	}
	return parts, outputNames, condArgs, nil
}

// mergeWhereClauses fusiona la cláusula estática (condiciones de listado) y la
// cláusula de búsqueda del usuario en UNA sola, para el único " WHERE " del
// fromWhere. Ambas no vacías ⇒ "(estáticas) AND (búsqueda)" (se parentizan los
// dos lados para que la precedencia AND/OR sea inequívoca: las estáticas pueden
// contener IN(...) y la búsqueda une sus fragmentos con AND). Solo una no vacía
// ⇒ esa tal cual. Ninguna ⇒ "" (sin WHERE; SQL idéntico a hoy).
func mergeWhereClauses(static, search string) string {
	switch {
	case static != "" && search != "":
		return "(" + static + ") AND (" + search + ")"
	case static != "":
		return static
	case search != "":
		return search
	default:
		return ""
	}
}

// buildActionFilterWhere compone la cláusula WHERE parametrizada a partir de los
// campos LLENOS de una acción SELECT. Recorre act.InputFields EN ORDEN DECLARADO
// (determinismo, no el mapa input) para que COUNT y SELECT emitan los $N en la
// misma secuencia. Por cada campo resuelve su tabla efectiva (N=1 ⇒ tabla
// principal, columna pelada; multi ⇒ f.SourceTable o la principal, columna
// cualificada "tabla"."col") y valida la columna contra la lista blanca de esa
// tabla con checkColumnWhitelisted; fuera de lista ⇒ ErrWhitelist SIN producir
// SQL. Si el valor en input[f.Name] es vacío/solo-espacios ⇒ se OMITE (sin
// predicado). exact (y cualquier MatchType no reconocido) ⇒
// lower(btrim(col::text)) = $n con arg lower+trim; partial ⇒
// lower(col::text) LIKE '%' || lower($n) || '%' con arg solo-trim (los comodines
// '%' son literales SQL, NUNCA interpolados ni parte del valor). Los fragmentos
// se unen con ' AND '; sin fragmentos ⇒ ("", nil, nil). Numera los $N desde
// startParam.
func buildActionFilterWhere(
	whitelist map[string][]string,
	cols []modules.ResolvedColumn,
	spec modules.MultiTableSpec,
	act modules.Action,
	input map[string]any,
	startParam int,
) (string, []any, error) {
	multi := spec.IsMultiTable()

	fragments := make([]string, 0, len(act.InputFields))
	args := make([]any, 0, len(act.InputFields))
	n := startParam
	for _, f := range act.InputFields {
		// Tabla efectiva: N=1 ⇒ la principal (f.SourceTable vale "" y no cualifica);
		// multi ⇒ f.SourceTable declarada o, en su defecto, la principal.
		effectiveTable := spec.PrincipalTable
		if multi && f.SourceTable != "" {
			effectiveTable = f.SourceTable
		}

		// Validar la columna contra la lista blanca de su tabla efectiva ANTES de
		// componer. Fuera de lista ⇒ ErrWhitelist sin SQL.
		if err := checkColumnWhitelisted(whitelist, effectiveTable, f.Name); err != nil {
			return "", nil, err
		}

		// Valor vacío/solo-espacios (incluye el centinela "sin filtro" que
		// normaliza a vacío tras TrimSpace): se omite, no añade predicado.
		raw := fmt.Sprintf("%v", input[f.Name])
		if _, present := input[f.Name]; !present {
			raw = ""
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}

		// Cualificador de columna: N=1 pelado; multi "tabla"."col".
		var col string
		if multi {
			col = quoteIdent(effectiveTable) + "." + quoteIdent(f.Name)
		} else {
			col = quoteIdent(f.Name)
		}

		// Co-emisión (fragmento SQL, arg). El valor SIEMPRE viaja como parámetro
		// ligado $N; los comodines '%' son literales SQL concatenados.
		switch f.MatchType {
		case modules.SearchMatchPartial:
			// Insensible a mayúsculas en ambos lados vía lower(); el arg va sin
			// pre-lower en Go (lo normaliza el lower($N) de SQL).
			fragments = append(fragments, fmt.Sprintf("lower(%s::text) LIKE '%%' || lower($%d) || '%%'", col, n))
			args = append(args, strings.TrimSpace(raw))
		default:
			// "exact" (y cualquier valor no reconocido): misma normalización
			// lower+btrim que las condiciones de listado; el arg llega ya en
			// minúsculas desde Go.
			fragments = append(fragments, fmt.Sprintf("lower(btrim(%s::text)) = $%d", col, n))
			args = append(args, strings.ToLower(strings.TrimSpace(raw)))
		}
		n++
	}

	// Sin fragmentos ⇒ sin WHERE de filtro (el GET /list base queda idéntico).
	if len(fragments) == 0 {
		return "", nil, nil
	}
	return strings.Join(fragments, " AND "), args, nil
}

// checkColumnWhitelisted valida que la columna esté en la lista blanca de la
// tabla indicada; fuera de lista ⇒ ErrWhitelist (sin revelar detalles al
// cliente; el handler traduce a un mensaje neutro).
func checkColumnWhitelisted(whitelist map[string][]string, table, column string) error {
	wc, ok := whitelist[table]
	if !ok {
		return fmt.Errorf("%w: tabla %q", ErrWhitelist, table)
	}
	for _, c := range wc {
		if c == column {
			return nil
		}
	}
	return fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, column, table)
}

// qualifiedColumn es una columna cualificada por su tabla para el SELECT
// multi-tabla.
type qualifiedColumn struct {
	table  string
	column string
}

// alias devuelve el alias de salida <tabla>__<col> bajo el que la columna viaja
// en la fila (coherente con el frontend y el motor de escritura).
func (q qualifiedColumn) alias() string {
	return q.table + "__" + q.column
}

// selectColumnsMulti calcula las columnas (tabla, columna) a traer en el SELECT
// multi-tabla: la UNIÓN de (a) columnas de visualización de TODAS las tablas,
// (b) columnas PK de TODAS las tablas aunque visible==false (necesarias para
// dirigir escrituras), y (c) columnas de las condiciones de listado. Cada una se
// valida contra la lista blanca de SU tabla; fuera de lista ⇒ ErrWhitelist sin
// ejecutar SQL. Resultado en orden estable, sin duplicados. Las columnas de
// filtro de una acción SELECT NO necesitan proyectarse: el WHERE solo exige que
// la columna esté en la lista blanca, no en la proyección.
func selectColumnsMulti(whitelist map[string][]string, cols []modules.ResolvedColumn, spec modules.MultiTableSpec, conditions []modules.ListCondition) ([]qualifiedColumn, error) {
	allowed := func(table string) (map[string]struct{}, bool) {
		wc, ok := whitelist[table]
		if !ok {
			return nil, false
		}
		set := make(map[string]struct{}, len(wc))
		for _, c := range wc {
			set[c] = struct{}{}
		}
		return set, true
	}

	type key struct{ t, c string }
	seen := make(map[key]struct{})
	out := make([]qualifiedColumn, 0, len(cols))
	add := func(table, column string) error {
		set, ok := allowed(table)
		if !ok {
			return fmt.Errorf("%w: tabla %q", ErrWhitelist, table)
		}
		if _, ok := set[column]; !ok {
			return fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, column, table)
		}
		k := key{table, column}
		if _, dup := seen[k]; dup {
			return nil
		}
		seen[k] = struct{}{}
		out = append(out, qualifiedColumn{table: table, column: column})
		return nil
	}

	// (a) visualización + (b) PK aunque no visible.
	for _, c := range cols {
		if !c.Visible && !c.IsPrimaryKey {
			continue
		}
		if err := add(c.SourceTable, c.Field); err != nil {
			return nil, err
		}
	}
	// Asegura que TODA PK esté incluida aunque el filtro anterior la excluyera.
	for _, c := range cols {
		if c.IsPrimaryKey {
			if err := add(c.SourceTable, c.Field); err != nil {
				return nil, err
			}
		}
	}

	// (c) columnas de condiciones, cualificadas por su tabla.
	for _, cond := range conditions {
		if cond.Field == "" {
			continue
		}
		table := resolveConditionTable(cond, cols, spec)
		if err := add(table, cond.Field); err != nil {
			return nil, err
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].table != out[j].table {
			return out[i].table < out[j].table
		}
		return out[i].column < out[j].column
	})
	return out, nil
}

// resolveConditionTable determina la tabla de una condición de listado: (1) su
// SourceTable si está declarada; (2) la sourceTable de la columna homónima si es
// única; (3) la principal en otro caso. Save-time ya rechazó las homónimas sin
// declarar, así que este fallback es seguro.
func resolveConditionTable(cond modules.ListCondition, cols []modules.ResolvedColumn, spec modules.MultiTableSpec) string {
	if cond.SourceTable != "" {
		return cond.SourceTable
	}
	for _, c := range cols {
		if c.Field == cond.Field {
			return c.SourceTable
		}
	}
	return spec.PrincipalTable
}

// buildJoinClause compone "FROM <principal> [<JOIN> <tabla> ON ...]" recorriendo
// las relaciones en orden estable. La primera tabla es la principal; cada JOIN
// añade la tabla aún no incorporada de la relación. El tipo de JOIN se
// normaliza a INNER/LEFT (nunca se interpola el literal crudo). Todas las
// columnas del ON se validan contra la lista blanca de su tabla ANTES de
// componer. La dirección FK solo decide qué columna va a cada lado del ON.
//
// Además de los pares columna=columna (On[]), cada relación puede llevar
// condiciones constantes (OnConstants[]) "<tabla>.<col> = $N" cuyo valor se liga
// como parámetro (NUNCA se interpola). Esos parámetros se numeran desde
// startParam y se devuelven en args en el orden estable de emisión de los JOIN,
// de modo que SELECT de datos y COUNT compartan los mismos $N. Devuelve el FROM,
// los args del ON y el error.
func buildJoinClause(whitelist map[string][]string, spec modules.MultiTableSpec, startParam int) (string, []any, error) {
	allowed := func(table, column string) error {
		wc, ok := whitelist[table]
		if !ok {
			return fmt.Errorf("%w: tabla %q", ErrWhitelist, table)
		}
		for _, c := range wc {
			if c == column {
				return nil
			}
		}
		return fmt.Errorf("%w: columna de unión %q en la tabla %q", ErrWhitelist, column, table)
	}

	// n numera los parámetros ligados del ON; args los acumula en orden de emisión.
	n := startParam
	var args []any

	var sb strings.Builder
	sb.WriteString("FROM ")
	sb.WriteString(quoteIdent(spec.PrincipalTable))

	included := map[string]struct{}{spec.PrincipalTable: {}}

	// Recorre relaciones en orden estable (por (child, parent)) e incorpora cada
	// tabla no incluida aún. Se repasa hasta estabilizar para soportar cadenas.
	rels := append([]modules.Relationship(nil), spec.Relationships...)
	sort.SliceStable(rels, func(i, j int) bool {
		if rels[i].ChildTable != rels[j].ChildTable {
			return rels[i].ChildTable < rels[j].ChildTable
		}
		return rels[i].ParentTable < rels[j].ParentTable
	})

	remaining := make([]modules.Relationship, len(rels))
	copy(remaining, rels)
	for len(remaining) > 0 {
		progressed := false
		next := remaining[:0]
		for _, r := range remaining {
			_, hasChild := included[r.ChildTable]
			_, hasParent := included[r.ParentTable]
			// Solo se puede unir si exactamente una de las dos tablas ya está
			// incluida (la otra es la que se incorpora).
			switch {
			case hasChild && hasParent:
				// Ambas incluidas: la relación solo añade condición ON a un JOIN ya
				// emitido no es soportado en v1; se omite para no duplicar tablas.
				progressed = true
			case hasChild || hasParent:
				newTable := r.ChildTable
				if hasChild {
					newTable = r.ParentTable
				}
				joinKw := "INNER JOIN"
				if modulesNormalizeJoin(r.JoinType) == modules.JoinLeft {
					joinKw = "LEFT JOIN"
				}
				onParts := make([]string, 0, len(r.On)+len(r.OnConstants))
				for _, p := range r.On {
					if err := allowed(r.ChildTable, p.Child); err != nil {
						return "", nil, err
					}
					if err := allowed(r.ParentTable, p.Parent); err != nil {
						return "", nil, err
					}
					onParts = append(onParts, fmt.Sprintf(
						"%s.%s = %s.%s",
						quoteIdent(r.ChildTable), quoteIdent(p.Child),
						quoteIdent(r.ParentTable), quoteIdent(p.Parent),
					))
				}
				if len(onParts) == 0 {
					return "", nil, fmt.Errorf("%w: la relación %q→%q no declara columnas de unión", ErrWhitelist, r.ChildTable, r.ParentTable)
				}
				// Constantes del ON: "<tabla>"."<col>" = $N con el valor ligado. La
				// tabla debe ser una de las dos de la relación; la columna se valida
				// contra la lista blanca. El valor NUNCA se interpola.
				for _, jc := range r.OnConstants {
					if jc.Table != r.ChildTable && jc.Table != r.ParentTable {
						return "", nil, fmt.Errorf("%w: la constante de unión referencia la tabla %q ajena a la relación %q→%q",
							ErrWhitelist, jc.Table, r.ChildTable, r.ParentTable)
					}
					if err := allowed(jc.Table, jc.Column); err != nil {
						return "", nil, err
					}
					onParts = append(onParts, fmt.Sprintf("%s.%s = $%d",
						quoteIdent(jc.Table), quoteIdent(jc.Column), n))
					args = append(args, jc.Value) // valor ligado; Postgres coacciona string→numérico/texto
					n++
				}
				sb.WriteString(" ")
				sb.WriteString(joinKw)
				sb.WriteString(" ")
				sb.WriteString(quoteIdent(newTable))
				sb.WriteString(" ON ")
				sb.WriteString(strings.Join(onParts, " AND "))
				included[newTable] = struct{}{}
				progressed = true
			default:
				// Ninguna incluida aún: se pospone.
				next = append(next, r)
			}
		}
		if !progressed {
			// No se pudo incorporar ninguna relación (grafo desconectado de la
			// principal): se aborta sin ejecutar.
			return "", nil, fmt.Errorf("%w: hay tablas no enlazadas con la principal", ErrWhitelist)
		}
		remaining = next
	}

	return sb.String(), args, nil
}

// modulesNormalizeJoin expone la normalización de joinType del paquete modules
// sin exportar la función interna; aquí se decide INNER/LEFT.
func modulesNormalizeJoin(jt string) string {
	if jt == modules.JoinLeft {
		return modules.JoinLeft
	}
	return modules.JoinInner
}

// Exec ejecuta una Acción de escritura (INSERT/UPDATE/DELETE) construida con la
// lista blanca y parámetros ligados. El input ya viene validado por el router.
func (e *PGExecutor) Exec(ctx context.Context, def modules.Definition, country string, act modules.Action, input map[string]any) (any, string, error) {
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)

	// Materializa los valores constantes declarados por la acción ANTES de
	// construir la SQL, para que una columna objetivo con origen "constant" (p. ej.
	// "Inactivar" que fija estado=2) entre en el SET/INSERT aunque el usuario no la
	// teclee. Devuelve un mapa nuevo; no muta el input recibido.
	input = applyConstantFields(act, input)

	if !spec.IsMultiTable() {
		// Camino N=1. INV-3: una acción INSERT con TargetTable y estrategias de
		// llenado DEBE resolverse dentro de una transacción (los SELECT de
		// catálogo/salto comparten snapshot). El guardado garantiza que una acción
		// con estrategias declara TargetTable; aun así, si llega esa combinación en
		// N=1 (config consistente sin tablas foráneas) se abre la tx aquí.
		op := strings.ToUpper(strings.TrimSpace(act.SQLOp))
		if op == "INSERT" && act.TargetTable != "" && needsResolutionTx(act) {
			return e.execInsertWithStrategies(ctx, def, spec, act, input)
		}
		// --- Camino N=1 byte-por-byte: una sentencia, SIN transacción ---
		table := stringFromConfig(def.FetchMapping, "table", "")
		if err := checkTable(def.DataSource.PGWhitelist, table); err != nil {
			return nil, "", err
		}
		cols := modules.ColumnsFromDisplayMapping(def.DisplayMapping)
		query, args, err := buildWriteSQL(def.DataSource.PGWhitelist, table, cols, act, input)
		if err != nil {
			return nil, "", err
		}
		db, err := e.pool(ctx, def)
		if err != nil {
			return nil, "", err
		}
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, "", fmt.Errorf("engine: error ejecutando la operación: %w", err)
		}
		affected, _ := res.RowsAffected()
		return map[string]any{"rowsAffected": affected}, inferObjectID(input), nil
	}

	// --- Camino multi-tabla: UNA transacción, escritura dirigida por tabla ---
	cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, spec.PrincipalTable)
	db, err := e.pool(ctx, def)
	if err != nil {
		return nil, "", err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("engine: no se pudo abrir la transacción: %w", err)
	}

	var affected int64
	op := strings.ToUpper(strings.TrimSpace(act.SQLOp))
	switch op {
	case "UPDATE":
		affected, err = execMultiUpdate(ctx, tx, def.DataSource.PGWhitelist, cols, spec, act, input)
	case "DELETE":
		affected, err = execMultiDelete(ctx, tx, def.DataSource.PGWhitelist, cols, spec, act, input)
	case "INSERT":
		insertInput := input
		if needsResolutionTx(act) {
			// Resuelve las estrategias de llenado ANTES del INSERT, dentro de la
			// MISMA tx (INV-3). Sin estrategias, insertInput es el input tal cual
			// (byte-por-byte como hoy, INV-9). execSingleInsert/buildWriteSQL no cambian.
			insertInput, err = resolveFillStrategies(ctx, tx, def.DataSource.PGWhitelist, act, insertInput)
		}
		if err == nil {
			// Pre-check de unicidad (§8) tras resolver estrategias y ANTES del INSERT.
			// No-op si act.Uniqueness == nil.
			err = checkUniqueness(ctx, tx, def.DataSource.PGWhitelist, act, insertInput)
		}
		if err == nil {
			affected, err = execSingleInsert(ctx, tx, def.DataSource.PGWhitelist, cols, spec, act, insertInput)
		}
	default:
		err = fmt.Errorf("engine: operación SQL no soportada %q", op)
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, "", err
	}
	if cerr := tx.Commit(); cerr != nil {
		return nil, "", fmt.Errorf("engine: no se pudo confirmar la transacción: %w", cerr)
	}
	return map[string]any{"rowsAffected": affected}, inferObjectID(input), nil
}

// txExecer abstrae ExecContext/QueryContext para que execMulti* y
// resolveFillStrategies operen sobre *sql.Tx (o un doble en pruebas). La
// ampliación con QueryContext es ADITIVA: *sql.Tx ya satisface ambos métodos.
type txExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// pkValuePresent aplica la definición EXACTA de "presente" del valor de PK en
// multi-tabla: la clave existe, su valor NO es nil y su texto NO queda vacío tras
// TrimSpace. Así 0, "0", false SON válidos; solo nil y la cadena vacía/espacios
// cuentan como ausente.
func pkValuePresent(input map[string]any, aliasKey string) bool {
	v, ok := input[aliasKey]
	if !ok || v == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v)) != ""
}

// execMultiUpdate ejecuta un UPDATE dirigido por tabla (§5.2). Orden de fases
// crítico: (1) resolver setCols por tabla (objetivo ∩ input ∩ whitelist),
// descartando las tablas con setCols vacío (R9.7); (2) exigir y leer la PK SOLO
// de las tablas afectadas por su alias <T>__<k>, abortando (0 sentencias) si
// falta cualquiera; (3) emitir un UPDATE por tabla afectada. Reindexado
// alias→valor para reutilizar buildUpdate con el nombre REAL de la PK.
func execMultiUpdate(ctx context.Context, tx txExecer, whitelist map[string][]string, cols []modules.ResolvedColumn, spec modules.MultiTableSpec, act modules.Action, input map[string]any) (int64, error) {
	byTable := modules.ActionWriteColumnsByTable(cols, act)
	pkByTable := modules.PrimaryKeyColumnsByTable(cols)

	type affectedTable struct {
		table   string
		setCols []string
	}

	// Fase 1: setCols por tabla (objetivo ∩ input ∩ whitelist). 0 SQL todavía.
	tables := make([]string, 0, len(byTable))
	for t := range byTable {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	affected := make([]affectedTable, 0, len(tables))
	for _, t := range tables {
		allowedSet := make(map[string]struct{}, len(whitelist[t]))
		for _, c := range whitelist[t] {
			allowedSet[c] = struct{}{}
		}
		setCols := make([]string, 0, len(byTable[t]))
		for _, c := range byTable[t] {
			if _, ok := input[c]; !ok {
				continue
			}
			if _, ok := allowedSet[c]; !ok {
				return 0, fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, c, t)
			}
			setCols = append(setCols, c)
		}
		sort.Strings(setCols)
		if len(setCols) == 0 {
			continue // tabla sin columnas a modificar: se OMITE (R9.7)
		}
		affected = append(affected, affectedTable{table: t, setCols: setCols})
	}

	// Fase 2: exigir PK SOLO de las tablas afectadas (por alias <T>__<k>).
	for _, a := range affected {
		pks := pkByTable[a.table]
		if len(pks) == 0 {
			return 0, fmt.Errorf("engine: la tabla %q no declara llave primaria para el UPDATE", a.table)
		}
		allowedSet := make(map[string]struct{}, len(whitelist[a.table]))
		for _, c := range whitelist[a.table] {
			allowedSet[c] = struct{}{}
		}
		for _, k := range pks {
			if _, ok := allowedSet[k]; !ok {
				return 0, fmt.Errorf("%w: columna clave %q en la tabla %q", ErrWhitelist, k, a.table)
			}
			aliasKey := a.table + "__" + k
			if !pkValuePresent(input, aliasKey) {
				return 0, fmt.Errorf("engine: UPDATE requiere el valor de la clave %q en la tabla %q", k, a.table)
			}
		}
	}

	// Fase 3: un UPDATE por tabla afectada, con el localInput reindexado.
	var total int64
	for _, a := range affected {
		pks := pkByTable[a.table]
		localInput := make(map[string]any, len(a.setCols)+len(pks))
		for _, c := range a.setCols {
			localInput[c] = input[c]
		}
		for _, k := range pks {
			localInput[k] = input[a.table+"__"+k]
		}
		q, args, err := buildUpdate(quoteIdent(a.table), a.setCols, pks, localInput)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return 0, fmt.Errorf("engine: error ejecutando la operación: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// execMultiDelete ejecuta un DELETE dirigido por el conjunto ordenado hijo→padre
// (§5.3). TODA tabla del conjunto exige PK con valor presente (asimetría
// intencional con el UPDATE: no hay tablas omitibles). Fase de validación sin
// emitir SQL; si falta cualquier PK ⇒ aborta (0 sentencias). Luego un DELETE por
// tabla en orden, con el nombre REAL de la PK.
func execMultiDelete(ctx context.Context, tx txExecer, whitelist map[string][]string, cols []modules.ResolvedColumn, spec modules.MultiTableSpec, act modules.Action, input map[string]any) (int64, error) {
	tree, err := modules.BuildRelationTree(spec)
	if err != nil {
		return 0, err
	}
	top := act.DeleteTopTable
	if top == "" {
		top = spec.PrincipalTable
	}
	order, err := tree.DeleteOrder(top)
	if err != nil {
		return 0, err
	}
	pkByTable := modules.PrimaryKeyColumnsByTable(cols)

	// Fase 1: validación — 0 sentencias si falta algo.
	for _, t := range order {
		pks := pkByTable[t]
		if len(pks) == 0 {
			return 0, fmt.Errorf("engine: la tabla %q del conjunto de borrado no declara llave primaria", t)
		}
		allowedSet := make(map[string]struct{}, len(whitelist[t]))
		for _, c := range whitelist[t] {
			allowedSet[c] = struct{}{}
		}
		for _, k := range pks {
			if _, ok := allowedSet[k]; !ok {
				return 0, fmt.Errorf("%w: columna clave %q en la tabla %q", ErrWhitelist, k, t)
			}
			aliasKey := t + "__" + k
			if !pkValuePresent(input, aliasKey) {
				return 0, fmt.Errorf("engine: DELETE requiere el valor de la clave %q en la tabla %q", k, t)
			}
		}
	}

	// Fase 2: un DELETE por tabla en orden hijo→padre.
	var total int64
	for _, t := range order {
		pks := pkByTable[t]
		localInput := make(map[string]any, len(pks))
		for _, k := range pks {
			localInput[k] = input[t+"__"+k]
		}
		q, args, err := buildDelete(quoteIdent(t), pks, localInput)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return 0, fmt.Errorf("engine: error ejecutando la operación: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// execSingleInsert ejecuta un INSERT a una sola tabla (act.TargetTable o la
// principal). Reutiliza la rama INSERT de buildWriteSQL con esa tabla y su lista
// blanca. No requiere PK; sin JOIN. Corre dentro de la tx del despacho
// multi-tabla por uniformidad.
func execSingleInsert(ctx context.Context, tx txExecer, whitelist map[string][]string, cols []modules.ResolvedColumn, spec modules.MultiTableSpec, act modules.Action, input map[string]any) (int64, error) {
	target := act.TargetTable
	if target == "" {
		target = spec.PrincipalTable
	}
	if err := checkTable(whitelist, target); err != nil {
		return 0, err
	}
	// Solo las columnas de la tabla objetivo: el INSERT no usa PK de otras tablas
	// y buildWriteSQL valida las PK contra la lista blanca de la tabla objetivo.
	targetCols := make([]modules.ResolvedColumn, 0, len(cols))
	for _, c := range cols {
		if c.SourceTable == target {
			targetCols = append(targetCols, c)
		}
	}
	q, args, err := buildWriteSQL(whitelist, target, targetCols, act, input)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		// Carrera (hallazgo 9/§8): el pre-check de unicidad pudo pasar y un INSERT
		// concurrente ganar; si la BD tiene un UNIQUE real, Postgres devuelve 23505.
		// Se mapea al MISMO error de negocio (mensaje del admin, nunca SQL).
		if act.Uniqueness != nil && isUniqueViolation(err) {
			return 0, fmt.Errorf("%w: %s", ErrUniqueness, uniquenessMessage(act.Uniqueness))
		}
		return 0, fmt.Errorf("engine: error ejecutando la operación: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// buildWriteSQL compone el SQL parametrizado de una acción de escritura. Valida
// CADA columna del input contra la lista blanca de la tabla; un identificador
// fuera de lista aborta con ErrWhitelist SIN ejecutar. Los valores del usuario
// jamás se interpolan en el texto: solo viajan como argumentos $N (Req. 4.4,
// 4.5, 4.6). Las columnas se recorren en orden estable para un SQL determinista.
func buildWriteSQL(whitelist map[string][]string, table string, defCols []modules.ResolvedColumn, act modules.Action, input map[string]any) (string, []any, error) {
	op := strings.ToUpper(strings.TrimSpace(act.SQLOp))
	allowed := whitelistedColumns(whitelist, table)
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, c := range allowed {
		allowedSet[c] = struct{}{}
	}

	// Columnas presentes en el input que correspondan a campos declarados,
	// recorridas en orden estable.
	cols := make([]string, 0, len(act.InputFields))
	for _, f := range act.InputFields {
		if _, ok := input[f.Name]; ok {
			cols = append(cols, f.Name)
		}
	}
	sort.Strings(cols)

	// Validación de lista blanca de cada columna referenciada.
	for _, c := range cols {
		if _, ok := allowedSet[c]; !ok {
			return "", nil, fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, c, table)
		}
	}

	// Columnas llave primaria (WHERE). Precedencia ÚNICA: se derivan de la
	// definición (DisplayColumn.IsPrimaryKey); si NINGUNA columna la declara
	// (definición antigua no re-guardada), se cae al fallback heredado isKey de la
	// acción. El camino legacy se activa SOLO por ausencia de PK de columna, nunca
	// porque las columnas editables resulten vacías.
	pkCols := modules.PrimaryKeyColumns(defCols)
	legacy := false
	if len(pkCols) == 0 {
		pkCols = legacyKeyColumns(act)
		legacy = true
	}
	for _, k := range pkCols {
		if _, ok := allowedSet[k]; !ok {
			return "", nil, fmt.Errorf("%w: columna clave %q en la tabla %q", ErrWhitelist, k, table)
		}
	}

	qt := quoteIdent(table)
	switch op {
	case "INSERT":
		if len(cols) == 0 {
			return "", nil, fmt.Errorf("engine: INSERT sin columnas")
		}
		placeholders := make([]string, len(cols))
		args := make([]any, len(cols))
		for i, c := range cols {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = input[c]
		}
		q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", qt, joinQuotedIdents(cols), strings.Join(placeholders, ", "))
		return q, args, nil
	case "UPDATE":
		// SET: si legacy, comportamiento viejo EXACTO (columnas del input que no
		// son clave). Si hay PK de columna, el SET son las columnas editables
		// (no-PK) presentes en el input, validadas contra la lista blanca.
		var setCols []string
		if legacy {
			setCols = cols
		} else {
			// El SET lo decide la ACCIÓN: solo sus columnas objetivo (ActionWriteColumns
			// ya aplica el techo editable no-PK) que además estén presentes en el input.
			// Una acción sin InputFields ⇒ todas las editables (compatibilidad, R8.2).
			setCols = intersectSorted(modules.ActionWriteColumns(defCols, act), input)
			for _, c := range setCols {
				if _, ok := allowedSet[c]; !ok {
					return "", nil, fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, c, table)
				}
			}
		}
		return buildUpdate(qt, setCols, pkCols, input)
	case "DELETE":
		return buildDelete(qt, pkCols, input)
	default:
		return "", nil, fmt.Errorf("engine: operación SQL no soportada %q", op)
	}
}

// applyConstantFields fusiona en el input los valores constantes declarados por
// la acción: por cada InputField con Source=="constant", fija input[f.Name] =
// f.ConstValue si esa clave no viene ya en el input. Devuelve un mapa NUEVO (no
// muta el recibido). Es coherente con ValidateInput, que SALTA los campos
// constant (su valor es literal y fijo, no lo teclea el usuario). El valor se
// envía como string (parámetro ligado); Postgres lo coacciona al tipo de la
// columna (p. ej. '2' a integer).
func applyConstantFields(act modules.Action, input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+len(act.InputFields))
	for k, v := range input {
		out[k] = v
	}
	for _, f := range act.InputFields {
		if f.Source != modules.FieldSourceConstant {
			continue
		}
		if _, ok := out[f.Name]; ok {
			continue // ya presente: no se pisa
		}
		out[f.Name] = f.ConstValue
	}
	return out
}

// legacyKeyColumns devuelve, en orden estable, los nombres de los campos de la
// acción marcados como clave del WHERE (IsKey). Es el fallback DEPRECADO para
// definiciones antiguas que aún no declaran columnas llave primaria; ver el
// comentario de modules.FieldSpec.IsKey.
func legacyKeyColumns(act modules.Action) []string {
	keys := make([]string, 0)
	for _, f := range act.InputFields {
		if f.IsKey {
			keys = append(keys, f.Name)
		}
	}
	sort.Strings(keys)
	return keys
}

// intersectSorted devuelve, en orden estable, los nombres de `candidates` que
// están presentes como clave en `input`.
func intersectSorted(candidates []string, input map[string]any) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := input[c]; ok {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// buildUpdate compone UPDATE tabla SET (cols no-clave) WHERE (cols clave). Las
// columnas clave vienen de los campos marcados IsKey (clave simple o compuesta,
// combinada con AND). Requiere al menos una columna clave y al menos una columna
// a modificar; todos los valores viajan como parámetros ligados.
func buildUpdate(qt string, cols, keyCols []string, input map[string]any) (string, []any, error) {
	if len(keyCols) == 0 {
		return "", nil, fmt.Errorf("engine: UPDATE requiere al menos una columna llave primaria")
	}
	keySet := make(map[string]struct{}, len(keyCols))
	for _, k := range keyCols {
		keySet[k] = struct{}{}
		if _, ok := input[k]; !ok {
			return "", nil, fmt.Errorf("engine: UPDATE requiere el valor de la clave %q", k)
		}
	}

	sets := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols)+len(keyCols))
	n := 1
	for _, c := range cols {
		if _, isKey := keySet[c]; isKey {
			continue // las columnas clave van en el WHERE, no en el SET
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", quoteIdent(c), n))
		args = append(args, input[c])
		n++
	}
	if len(sets) == 0 {
		return "", nil, fmt.Errorf("engine: UPDATE sin columnas a modificar")
	}

	where := make([]string, 0, len(keyCols))
	for _, k := range keyCols {
		where = append(where, fmt.Sprintf("%s = $%d", quoteIdent(k), n))
		args = append(args, input[k])
		n++
	}

	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s", qt, strings.Join(sets, ", "), strings.Join(where, " AND "))
	return q, args, nil
}

// buildDelete compone DELETE FROM tabla WHERE (cols clave). Las columnas clave
// vienen de los campos marcados IsKey (clave simple o compuesta, combinada con
// AND). Requiere al menos una columna clave y su valor; todos los valores van
// como parámetros ligados.
func buildDelete(qt string, keyCols []string, input map[string]any) (string, []any, error) {
	if len(keyCols) == 0 {
		return "", nil, fmt.Errorf("engine: DELETE requiere al menos una columna llave primaria")
	}
	where := make([]string, 0, len(keyCols))
	args := make([]any, 0, len(keyCols))
	n := 1
	for _, k := range keyCols {
		val, ok := input[k]
		if !ok {
			return "", nil, fmt.Errorf("engine: DELETE requiere el valor de la clave %q", k)
		}
		where = append(where, fmt.Sprintf("%s = $%d", quoteIdent(k), n))
		args = append(args, val)
		n++
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", qt, strings.Join(where, " AND "))
	return q, args, nil
}

// checkTable valida que la tabla esté declarada en la lista blanca.
func checkTable(whitelist map[string][]string, table string) error {
	if table == "" {
		return fmt.Errorf("%w: no se declaró tabla", ErrWhitelist)
	}
	if _, ok := whitelist[table]; !ok {
		return fmt.Errorf("%w: tabla %q", ErrWhitelist, table)
	}
	return nil
}

// whitelistedColumns devuelve las columnas permitidas de una tabla, en orden
// estable.
func whitelistedColumns(whitelist map[string][]string, table string) []string {
	cols := append([]string(nil), whitelist[table]...)
	sort.Strings(cols)
	return cols
}

// needsResolutionTx indica si una acción INSERT necesita abrir la transacción de
// resolución (INV-3): cuando declara estrategias de llenado (HasFillStrategies) o
// cuando declara una verificación de unicidad (Uniqueness != nil). El pre-check
// de unicidad y los SELECT de catálogo/salto deben correr dentro de la MISMA
// transacción que el INSERT para compartir snapshot. La tabla objetivo
// (act.TargetTable != "") la comprueba el llamador.
func needsResolutionTx(act modules.Action) bool {
	return modules.HasFillStrategies(act) || act.Uniqueness != nil
}

// execInsertWithStrategies abre una transacción en el camino N=1 para resolver
// las estrategias de llenado y ejecutar el INSERT single-table dentro del mismo
// snapshot. Revierte ante cualquier error de resolución o de INSERT.
func (e *PGExecutor) execInsertWithStrategies(ctx context.Context, def modules.Definition, spec modules.MultiTableSpec, act modules.Action, input map[string]any) (any, string, error) {
	cols := modules.ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, spec.PrincipalTable)
	db, err := e.pool(ctx, def)
	if err != nil {
		return nil, "", err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("engine: no se pudo abrir la transacción: %w", err)
	}
	resolved, err := resolveFillStrategies(ctx, tx, def.DataSource.PGWhitelist, act, input)
	if err != nil {
		_ = tx.Rollback()
		return nil, "", err
	}
	// Pre-check de unicidad (§8) DESPUÉS de resolver las estrategias (los valores
	// derivados ya están en `resolved`) y ANTES del INSERT, dentro de la misma tx.
	if err := checkUniqueness(ctx, tx, def.DataSource.PGWhitelist, act, resolved); err != nil {
		_ = tx.Rollback()
		return nil, "", err
	}
	affected, err := execSingleInsert(ctx, tx, def.DataSource.PGWhitelist, cols, spec, act, resolved)
	if err != nil {
		_ = tx.Rollback()
		return nil, "", err
	}
	if cerr := tx.Commit(); cerr != nil {
		return nil, "", fmt.Errorf("engine: no se pudo confirmar la transacción: %w", cerr)
	}
	return map[string]any{"rowsAffected": affected}, inferObjectID(input), nil
}

// ErrDeriveRelation es el error de negocio que resolveFillStrategies devuelve
// (envuelto) cuando una estrategia de llenado no puede resolverse: un
// captureKey/sourceCaptureKey/localCaptureKey sin declarar, o una relectura de
// catálogo/salto que no devuelve EXACTAMENTE una fila (0 ⇒ no existe la relación;
// >1 ⇒ no es 1-a-1). El handler lo mapea a E422_DERIVE con un mensaje FIJO de
// negocio (sin concatenar err.Error()); el detalle queda solo en logs del motor.
var ErrDeriveRelation = errors.New("engine: no se pudo resolver la relación")

// ErrUniqueness es el error de negocio que el motor devuelve (envuelto con el
// mensaje del admin) cuando una acción INSERT viola la combinación de columnas
// declarada única: el pre-check SELECT encontró una fila existente, o el INSERT
// chocó con un UNIQUE real de la BD (SQLSTATE 23505). El handler lo mapea a
// 409 E409_UNIQUENESS mostrando el mensaje de negocio, NUNCA el SQL (design.md §8).
var ErrUniqueness = errors.New("engine: la combinación de valores ya existe")

// uniquenessMessage devuelve el mensaje de negocio a mostrar al chocar: el
// Message declarado por el admin o, si está vacío, uno genérico en español.
func uniquenessMessage(u *modules.UniquenessCheck) string {
	if u != nil && strings.TrimSpace(u.Message) != "" {
		return u.Message
	}
	return "Ya existe un registro con esa combinación de valores"
}

// buildUniquenessSQL compone el pre-check de unicidad: "SELECT 1 FROM <table>
// WHERE <pred> LIMIT 1". Por cada columna de la combinación, si el input
// resuelto tiene un valor NO nil usa "col = $N" (valor ligado); si la columna
// está ausente o su valor es nil usa "col IS NOT DISTINCT FROM $N" con NULL
// ligado, de modo que dos "sin valor" cuenten como iguales (design.md §8). CADA
// columna se valida contra la lista blanca de la tabla; fuera de lista ⇒
// ErrWhitelist SIN producir SQL. Sin columnas ⇒ error que envuelve ErrValidation.
// LIMIT 1 y los operadores son literales fijos; los valores jamás se interpolan.
func buildUniquenessSQL(whitelist map[string][]string, table string, columns []string, input map[string]any) (string, []any, error) {
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("%w: la verificación de unicidad requiere al menos una columna", ErrValidation)
	}
	if err := checkTable(whitelist, table); err != nil {
		return "", nil, err
	}
	allowedSet := make(map[string]struct{}, len(whitelist[table]))
	for _, c := range whitelist[table] {
		allowedSet[c] = struct{}{}
	}
	preds := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	n := 1
	for _, col := range columns {
		if _, ok := allowedSet[col]; !ok {
			return "", nil, fmt.Errorf("%w: columna de unicidad %q en la tabla %q", ErrWhitelist, col, table)
		}
		v, present := input[col]
		if !present || v == nil {
			// Ausente o NULL: ambos son "sin valor"; IS NOT DISTINCT FROM NULL casa
			// con filas cuyo valor también es NULL.
			preds = append(preds, fmt.Sprintf("%s IS NOT DISTINCT FROM $%d", quoteIdent(col), n))
			args = append(args, nil)
		} else {
			preds = append(preds, fmt.Sprintf("%s = $%d", quoteIdent(col), n))
			args = append(args, v)
		}
		n++
	}
	sql := fmt.Sprintf("SELECT 1 FROM %s WHERE %s LIMIT 1", quoteIdent(table), strings.Join(preds, " AND "))
	return sql, args, nil
}

// checkUniqueness ejecuta el pre-check de unicidad sobre la tx ANTES del INSERT.
// Si la combinación ya existe (el SELECT devuelve una fila) ⇒ ErrUniqueness
// envuelto con el mensaje de negocio del admin; el llamador revierte la tx y no
// inserta. Sin fila ⇒ nil (se procede al INSERT).
func checkUniqueness(ctx context.Context, tx txExecer, whitelist map[string][]string, act modules.Action, input map[string]any) error {
	if act.Uniqueness == nil {
		return nil
	}
	query, args, err := buildUniquenessSQL(whitelist, act.TargetTable, act.Uniqueness.Columns, input)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("engine: error verificando unicidad: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%w: %s", ErrUniqueness, uniquenessMessage(act.Uniqueness))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("engine: error verificando unicidad: %w", err)
	}
	return nil
}

// isUniqueViolation reconoce una violación de restricción UNIQUE de PostgreSQL
// por su SQLSTATE 23505 (unique_violation), sin parsear el texto del error. Se
// usa tras el ExecContext del INSERT para mapear una carrera (el pre-check pasó
// pero un INSERT concurrente ganó) al mismo ErrUniqueness de negocio (design.md §8).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// buildRowLookupSQL compone el SELECT acotado que relee una fila por una columna
// clave (catálogo o salto FK): "SELECT <selectColumns> FROM <table> WHERE
// <keyColumn> = $1 LIMIT 2". Calca la validación de buildOptionsSQL: valida la
// tabla con checkTable y CADA columna de selectColumns MÁS keyColumn contra la
// lista blanca; un identificador fuera de lista ⇒ ErrWhitelist SIN producir SQL.
// selectColumns vacío ⇒ error que envuelve ErrValidation. El LIMIT 2 es literal
// (detecta >1); el valor local lo liga el llamador como $1 vía
// lookupExactlyOneRow (este builder NO devuelve args: el valor no lo conoce).
func buildRowLookupSQL(whitelist map[string][]string, table, keyColumn string, selectColumns []string) (string, error) {
	if len(selectColumns) == 0 {
		return "", fmt.Errorf("%w: la relectura de fila requiere al menos una columna a proyectar", ErrValidation)
	}
	if err := checkTable(whitelist, table); err != nil {
		return "", err
	}
	allowedSet := make(map[string]struct{}, len(whitelist[table]))
	for _, c := range whitelist[table] {
		allowedSet[c] = struct{}{}
	}
	if _, ok := allowedSet[keyColumn]; !ok {
		return "", fmt.Errorf("%w: columna clave %q en la tabla %q", ErrWhitelist, keyColumn, table)
	}
	for _, col := range selectColumns {
		if _, ok := allowedSet[col]; !ok {
			return "", fmt.Errorf("%w: columna %q en la tabla %q", ErrWhitelist, col, table)
		}
	}
	sql := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s = $1 LIMIT 2",
		joinQuotedIdents(selectColumns), quoteIdent(table), quoteIdent(keyColumn),
	)
	return sql, nil
}

// lookupExactlyOneRow ejecuta un SELECT (ya compuesto por buildRowLookupSQL)
// sobre la tx, escanea N columnas arbitrarias con el patrón de scanRows
// (normalizeSQLValue) y exige EXACTAMENTE una fila. 0 filas (no existe), >1
// filas (no es 1-a-1) o cualquier error de escaneo ⇒ ErrDeriveRelation (el
// detalle real queda envuelto para el log). defer rows.Close() garantiza que la
// conexión de la tx no se filtra en ninguna rama.
func lookupExactlyOneRow(ctx context.Context, tx txExecer, query string, selectColumns []string, keyValue any) (map[string]any, error) {
	rows, err := tx.QueryContext(ctx, query, keyValue)
	if err != nil {
		return nil, fmt.Errorf("%w: error ejecutando la relectura: %v", ErrDeriveRelation, err)
	}
	defer rows.Close()

	all, err := scanRows(rows, selectColumns)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDeriveRelation, err)
	}
	if len(all) != 1 {
		return nil, fmt.Errorf("%w: se esperaba exactamente una fila, se obtuvieron %d", ErrDeriveRelation, len(all))
	}
	return all[0], nil
}

// resolveFillStrategies resuelve, dentro de la transacción del despacho, las
// estrategias de llenado de una acción INSERT y devuelve el input FINAL que
// alimenta execSingleInsert. Fases (design.md §5):
//
//  1. capture-from-catalog: toma el id de input[name]; si StoreColumn==IDColumn
//     el valor es el id; en otro caso (o si alimenta derivaciones) relee la fila
//     del catálogo UNA vez y la cachea por CaptureKey; INV-5
//     input[name]=fila[StoreColumn].
//  2. relectura por CaptureKey (buildRowLookupSQL) exigiendo exactamente 1 fila.
//  3. derive-from-option: toma Attribute de la fila del catálogo capturado.
//  4. derive-via-fk: AGRUPA por salto (dedup), un SELECT por salto proyectando la
//     unión de atributos, exige 1 fila, escribe input[name]=fila[Mapping[0].Attribute];
//     valor local sin relectura si LocalAttribute == IDColumn de la captura.
//  5. constantes: ya aplicadas por applyConstantFields.
//  6. proyección final (INV-2): deja solo claves ∈ columnas(whitelist[TargetTable]).
//
// Cualquier referencia sin resolver o fila ≠ 1 ⇒ ErrDeriveRelation.
func resolveFillStrategies(ctx context.Context, tx txExecer, whitelist map[string][]string, act modules.Action, input map[string]any) (map[string]any, error) {
	target := act.TargetTable

	// Copia de trabajo (no muta el input recibido).
	out := make(map[string]any, len(input)+len(act.InputFields))
	for k, v := range input {
		out[k] = v
	}

	// Índice de capturas por CaptureKey (catálogo + id capturado + fila releída).
	captures := make(map[string]*captureState)

	// --- Fase 1: capture-from-catalog ---
	for _, f := range act.InputFields {
		if f.Source != modules.FieldSourceFillStrategy || f.Strategy == nil {
			continue
		}
		s := f.Strategy
		if s.Kind != modules.StrategyCaptureFromCatalog {
			continue
		}
		if s.Catalog == nil {
			return nil, fmt.Errorf("%w: la captura %q no declara catálogo", ErrDeriveRelation, f.Name)
		}
		idValue, present := out[f.Name]
		if !present || idValue == nil {
			return nil, fmt.Errorf("%w: la captura %q no recibió un id", ErrDeriveRelation, f.Name)
		}
		storeColumn := s.StoreColumn
		if storeColumn == "" {
			storeColumn = s.Catalog.IDColumn
		}
		if s.CaptureKey != "" {
			captures[s.CaptureKey] = &captureState{strategy: s, idValue: idValue}
		}
		// Si el valor a escribir es el propio id, no hace falta releer para la
		// escritura directa; la relectura por derivaciones se hace en la fase 2.
		if strings.HasPrefix(f.Name, modules.CaptureKeyPrefix) {
			// Captura que NO escribe a columna: su valor solo alimenta derivaciones.
			continue
		}
		if storeColumn == s.Catalog.IDColumn {
			out[f.Name] = idValue
			continue
		}
		row, err := reloadCatalogRow(ctx, tx, whitelist, captures, s)
		if err != nil {
			return nil, err
		}
		out[f.Name] = normalizeSQLValue(row[storeColumn])
	}

	// --- Fase 3: derive-from-option ---
	for _, f := range act.InputFields {
		if f.Source != modules.FieldSourceFillStrategy || f.Strategy == nil {
			continue
		}
		s := f.Strategy
		if s.Kind != modules.StrategyDeriveFromOption {
			continue
		}
		cap, ok := captures[s.SourceCaptureKey]
		if !ok {
			return nil, fmt.Errorf("%w: derive-from-option %q referencia el captureKey %q sin resolver", ErrDeriveRelation, f.Name, s.SourceCaptureKey)
		}
		row, err := reloadCatalogRow(ctx, tx, whitelist, captures, cap.strategy)
		if err != nil {
			return nil, err
		}
		out[f.Name] = normalizeSQLValue(row[s.Attribute])
	}

	// --- Fase 4: derive-via-fk (con dedup de saltos idénticos) ---
	type jumpKey struct {
		relatedTable, relatedColumn, localCaptureKey, localAttribute string
	}
	type jumpGroup struct {
		fields     []modules.FieldSpec
		attributes []string // unión de Mapping[0].Attribute (dedup)
	}
	groups := make(map[jumpKey]*jumpGroup)
	order := make([]jumpKey, 0)
	for _, f := range act.InputFields {
		if f.Source != modules.FieldSourceFillStrategy || f.Strategy == nil {
			continue
		}
		s := f.Strategy
		if s.Kind != modules.StrategyDeriveViaFK {
			continue
		}
		if s.Jump == nil || len(s.Mapping) != 1 {
			return nil, fmt.Errorf("%w: derive-via-fk %q tiene un salto/mapeo inválido", ErrDeriveRelation, f.Name)
		}
		k := jumpKey{s.Jump.RelatedTable, s.Jump.RelatedColumn, s.Jump.LocalCaptureKey, s.Jump.LocalAttribute}
		g, ok := groups[k]
		if !ok {
			g = &jumpGroup{}
			groups[k] = g
			order = append(order, k)
		}
		g.fields = append(g.fields, f)
		attr := s.Mapping[0].Attribute
		found := false
		for _, a := range g.attributes {
			if a == attr {
				found = true
				break
			}
		}
		if !found {
			g.attributes = append(g.attributes, attr)
		}
	}
	for _, k := range order {
		g := groups[k]
		// Valor local: sin relectura si LocalAttribute == IDColumn de la captura.
		cap, ok := captures[k.localCaptureKey]
		if !ok {
			return nil, fmt.Errorf("%w: derive-via-fk referencia el captureKey local %q sin resolver", ErrDeriveRelation, k.localCaptureKey)
		}
		var localValue any
		if cap.strategy.Catalog != nil && k.localAttribute == cap.strategy.Catalog.IDColumn {
			localValue = cap.idValue
		} else {
			row, err := reloadCatalogRow(ctx, tx, whitelist, captures, cap.strategy)
			if err != nil {
				return nil, err
			}
			localValue = normalizeSQLValue(row[k.localAttribute])
		}
		// UN SELECT por salto proyectando la unión de atributos del grupo.
		sort.Strings(g.attributes)
		query, err := buildRowLookupSQL(whitelist, k.relatedTable, k.relatedColumn, g.attributes)
		if err != nil {
			return nil, err
		}
		row, err := lookupExactlyOneRow(ctx, tx, query, g.attributes, localValue)
		if err != nil {
			return nil, err
		}
		for _, f := range g.fields {
			out[f.Name] = normalizeSQLValue(row[f.Strategy.Mapping[0].Attribute])
		}
	}

	// --- Fase 6: proyección final (INV-2) ---
	allowedSet := make(map[string]struct{})
	for _, c := range whitelistedColumns(whitelist, target) {
		allowedSet[c] = struct{}{}
	}
	for k := range out {
		if _, ok := allowedSet[k]; !ok {
			delete(out, k)
		}
	}
	return out, nil
}

// captureState guarda, por CaptureKey, el estado de una captura
// capture-from-catalog resuelta durante resolveFillStrategies: su estrategia, el
// id elegido (input[name]) y la fila releída del catálogo (nil hasta la primera
// relectura; se cachea para no releer dos veces el mismo catálogo).
type captureState struct {
	strategy *modules.FillStrategy
	idValue  any
	row      map[string]any
}

// reloadCatalogRow relee (y cachea en la captura) la fila del catálogo de una
// captura capture-from-catalog por su id, proyectando TODAS las columnas de la
// lista blanca de la tabla del catálogo (para que cualquier atributo derivado
// esté disponible). Exige exactamente una fila (ErrDeriveRelation en otro caso).
func reloadCatalogRow(ctx context.Context, tx txExecer, whitelist map[string][]string, captures map[string]*captureState, s *modules.FillStrategy) (map[string]any, error) {
	// Localiza la captura por su estrategia para cachear la fila (evita releer dos
	// veces el mismo catálogo).
	var cap *captureState
	for _, c := range captures {
		if c.strategy == s {
			cap = c
			break
		}
	}
	if cap != nil && cap.row != nil {
		return cap.row, nil
	}
	var idValue any
	if cap != nil {
		idValue = cap.idValue
	}
	if idValue == nil {
		return nil, fmt.Errorf("%w: la captura del catálogo %q no tiene id para releer", ErrDeriveRelation, s.Catalog.Table)
	}
	cols := whitelistedColumns(whitelist, s.Catalog.Table)
	query, err := buildRowLookupSQL(whitelist, s.Catalog.Table, s.Catalog.IDColumn, cols)
	if err != nil {
		return nil, err
	}
	row, err := lookupExactlyOneRow(ctx, tx, query, cols, idValue)
	if err != nil {
		return nil, err
	}
	if cap != nil {
		cap.row = row
	}
	return row, nil
}

// selectColumns calcula las columnas a traer en el SELECT de la Acción Listar:
// la UNIÓN de las columnas de visualización (lista blanca de la tabla) con las
// columnas referenciadas por las condiciones de listado. Así applyConditions
// puede evaluar columnas que no se muestran (p. ej. "eliminado") sin que la
// fila se descarte por falta de la clave.
//
// VALIDACIÓN DE SEGURIDAD: cada columna de condición DEBE estar en la lista
// blanca de la tabla; si no lo está, aborta con un error que envuelve
// ErrWhitelist (evita 0 filas silenciosas e inyección de identificadores). El
// resultado va en orden estable (sort) y sin duplicados.
// Las columnas de filtro de una acción SELECT NO se añaden a esta unión: el
// WHERE de filtro solo exige que la columna esté en la lista blanca, no que esté
// proyectada (puede referenciar una columna visible==false sin proyectarla).
func selectColumns(whitelist map[string][]string, table string, conditions []modules.ListCondition) ([]string, error) {
	allowed := whitelistedColumns(whitelist, table)
	set := make(map[string]struct{}, len(allowed))
	for _, c := range allowed {
		set[c] = struct{}{}
	}

	allowedSet := make(map[string]struct{}, len(whitelist[table]))
	for _, c := range whitelist[table] {
		allowedSet[c] = struct{}{}
	}

	for _, cond := range conditions {
		col := cond.Field
		if col == "" {
			continue
		}
		if _, ok := allowedSet[col]; !ok {
			return nil, fmt.Errorf("%w: columna de condición %q en la tabla %q", ErrWhitelist, col, table)
		}
		set[col] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// buildListConditionsWhere compone UNA cláusula WHERE parametrizada a partir de
// las condiciones fijas de listado del módulo, para aplicarla por igual al
// SELECT de datos y al COUNT(*). Reproduce EXACTAMENTE la semántica en memoria
// de rowMeetsAll: por cada condición con Values no vacío, el valor de la columna
// debe coincidir (texto, sin distinguir mayúsculas, recortado) con ALGUNO de los
// valores permitidos (OR dentro de la columna ⇒ IN); las condiciones distintas
// se combinan con AND. Las condiciones con Field vacío o Values vacío se saltan
// (igual que rowMeetsAll). El SQL emitido es lower(btrim(<col>::text)) IN (...)
// y cada valor viaja como parámetro ligado, normalizado con el MISMO lower+trim.
//
// VALIDACIÓN DE SEGURIDAD: cada columna de condición DEBE estar en la lista
// blanca de la tabla; si no lo está, aborta con un error que envuelve
// ErrWhitelist SIN producir SQL. El identificador se entrecomilla con quoteIdent
// (defensa en profundidad); los valores NUNCA se interpolan.
//
// Los marcadores ($N) se numeran a partir de startParam, de modo que el SELECT
// de datos pueda renumerar LIMIT/OFFSET DESPUÉS de los parámetros del WHERE.
// Devuelve clause vacío y args nil cuando no se emite ninguna condición.
func buildListConditionsWhere(whitelist map[string][]string, table string, conditions []modules.ListCondition, startParam int) (string, []any, error) {
	allowedSet := make(map[string]struct{}, len(whitelist[table]))
	for _, c := range whitelist[table] {
		allowedSet[c] = struct{}{}
	}

	fragments := make([]string, 0, len(conditions))
	args := make([]any, 0)
	n := startParam
	for _, cond := range conditions {
		col := cond.Field
		if col == "" || len(cond.Values) == 0 {
			continue // sin campo o sin valores: no restringe (paridad con rowMeetsAll)
		}
		if _, ok := allowedSet[col]; !ok {
			return "", nil, fmt.Errorf("%w: columna de condición %q en la tabla %q", ErrWhitelist, col, table)
		}
		placeholders := make([]string, len(cond.Values))
		for i, v := range cond.Values {
			placeholders[i] = fmt.Sprintf("$%d", n)
			args = append(args, strings.ToLower(strings.TrimSpace(v)))
			n++
		}
		fragments = append(fragments, fmt.Sprintf(
			"lower(btrim(%s::text)) IN (%s)",
			quoteIdent(col), strings.Join(placeholders, ", "),
		))
	}

	if len(fragments) == 0 {
		return "", nil, nil
	}
	return strings.Join(fragments, " AND "), args, nil
}

// buildListConditionsWhereMulti es la versión multi-tabla de
// buildListConditionsWhere: cualifica cada condición por su tabla
// (lower(btrim("<tabla>"."<col>"::text)) IN (...)). Resuelve la tabla con
// resolveConditionTable y valida cada columna contra la lista blanca de su
// tabla. Comparte la misma semántica (lower+trim, IN, AND entre condiciones) y
// la misma numeración de parámetros desde startParam que el camino N=1, de modo
// que el COUNT y el SELECT de datos usen el mismo WHERE y args.
func buildListConditionsWhereMulti(whitelist map[string][]string, cols []modules.ResolvedColumn, spec modules.MultiTableSpec, conditions []modules.ListCondition, startParam int) (string, []any, error) {
	allowedSet := func(table string) map[string]struct{} {
		set := make(map[string]struct{}, len(whitelist[table]))
		for _, c := range whitelist[table] {
			set[c] = struct{}{}
		}
		return set
	}

	fragments := make([]string, 0, len(conditions))
	args := make([]any, 0)
	n := startParam
	for _, cond := range conditions {
		if cond.Field == "" || len(cond.Values) == 0 {
			continue
		}
		table := resolveConditionTable(cond, cols, spec)
		if _, ok := allowedSet(table)[cond.Field]; !ok {
			return "", nil, fmt.Errorf("%w: columna de condición %q en la tabla %q", ErrWhitelist, cond.Field, table)
		}
		placeholders := make([]string, len(cond.Values))
		for i, v := range cond.Values {
			placeholders[i] = fmt.Sprintf("$%d", n)
			args = append(args, strings.ToLower(strings.TrimSpace(v)))
			n++
		}
		fragments = append(fragments, fmt.Sprintf(
			"lower(btrim(%s.%s::text)) IN (%s)",
			quoteIdent(table), quoteIdent(cond.Field), strings.Join(placeholders, ", "),
		))
	}

	if len(fragments) == 0 {
		return "", nil, nil
	}
	return strings.Join(fragments, " AND "), args, nil
}

// quoteIdent entrecomilla un identificador SQL de forma segura, duplicando las
// comillas dobles internas. Solo se aplica a identificadores YA validados contra
// la lista blanca (defensa en profundidad).
func quoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// joinQuotedIdents entrecomilla y une una lista de identificadores con comas.
func joinQuotedIdents(idents []string) string {
	quoted := make([]string, len(idents))
	for i, id := range idents {
		quoted[i] = quoteIdent(id)
	}
	return strings.Join(quoted, ", ")
}

// scanRows materializa el resultado de una consulta en filas (mapas columna→valor).
func scanRows(rows *sql.Rows, cols []string) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("engine: error leyendo fila: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = normalizeSQLValue(values[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engine: error iterando filas: %w", err)
	}
	return out, nil
}

// normalizeSQLValue convierte []byte (texto SQL) a string para que serialice
// como JSON legible; el resto se deja tal cual.
func normalizeSQLValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
