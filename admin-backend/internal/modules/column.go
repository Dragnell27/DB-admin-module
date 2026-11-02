package modules

import (
	"encoding/json"
	"errors"
	"sort"
)

// ErrNoPrimaryKey se devuelve al guardar un módulo con acciones de editar o
// eliminar pero sin ninguna columna marcada como llave primaria (necesaria para
// construir el WHERE de UPDATE/DELETE). Se traduce a E422_VALIDATION.
var ErrNoPrimaryKey = errors.New("modules: define al menos una columna como llave primaria para las acciones de editar o eliminar")

// FieldTypeKeyPassthrough es el Type con el que EditFieldSpecs deriva las
// columnas LLAVE PRIMARIA. Su literal NO coincide con ninguno de los case que
// engine.checkType valida ("string", "", "number", "boolean", "date"), por lo
// que checkType cae a su rama default («tipo desconocido: no se valida el
// tipo») y la PK solo se valida por PRESENCIA (Required:true), nunca por forma.
// Así, el valor de una PK tomado VERBATIM de la fila seleccionada —un float64
// para una PK numérica típica (p. ej. un id serial sin formato numérico en la
// UI), o una fecha/forma arbitraria— satisface la validación sin rechazarse con
// E422_VALIDATION. El chequeo de tipo por formato se reserva para las columnas
// EDITABLES, cuyos valores sí teclea el usuario en el formulario (R12.1/R12.2).
const FieldTypeKeyPassthrough = "keyPassthrough"

// SearchMatchExact y SearchMatchPartial son los dos tipos de coincidencia de un
// campo de una ACCIÓN SELECT (filtro de listado): "exact" (col = $n) o "partial"
// (col LIKE $n). Se declaran como constantes para no interpolar literales
// sueltos: el builder del WHERE de filtro (buildActionFilterWhere) solo conoce
// estas dos ramas y cualquier otro valor cae al default "exact" (nunca se
// interpola).
const (
	SearchMatchExact   = "exact"
	SearchMatchPartial = "partial"
)

// DisplayColumn describe una columna de la tabla del módulo (paso
// «Visualización»): el campo de origen, su etiqueta, orden y formato de
// presentación, más tres banderas de comportamiento. Es la forma TAL CUAL se
// serializa en displayMapping.columns (JSONB), por lo que Visible y Editable son
// punteros para distinguir "ausente" (definición antigua) de false.
//
//   - Visible: la columna se muestra como columna al listar. Ausente = true.
//   - IsPrimaryKey: la columna forma parte de la CLAVE del WHERE de
//     UPDATE/DELETE (varias marcadas = clave compuesta). Ausente = false.
//   - Editable: la columna puede modificarse (entra en el SET del UPDATE).
//     Ausente = true, salvo que la columna sea PK (que fuerza no editable).
type DisplayColumn struct {
	Field        string `json:"field"`
	Label        string `json:"label,omitempty"`
	Order        int    `json:"order,omitempty"`
	Format       string `json:"format,omitempty"`
	Visible      *bool  `json:"visible,omitempty"`
	IsPrimaryKey bool   `json:"isPrimaryKey,omitempty"`
	Editable     *bool  `json:"editable,omitempty"`
	// SourceTable es la tabla a la que pertenece la columna (multi-tabla).
	// Ausente ⇒ la tabla principal (compatibilidad N=1). Junto con Field forma la
	// identidad (sourceTable, field) que desambigua columnas homónimas.
	SourceTable string `json:"sourceTable,omitempty"`
}

// ResolvedColumn es una DisplayColumn con las banderas YA normalizadas a bool
// (sin punteros): el resto del código no vuelve a aplicar defaults ni la regla
// PK ⇒ no editable.
type ResolvedColumn struct {
	Field        string
	Label        string
	Order        int
	Format       string
	Visible      bool
	IsPrimaryKey bool
	Editable     bool
	// SourceTable es la tabla de la columna YA resuelta (principal si la
	// definición no la declara). En N=1 es CONSTANTE (la principal) en todas las
	// columnas.
	SourceTable string
}

// ColumnsFromDisplayMapping extrae, en orden estable por Order (y Field a
// igualdad), las columnas declaradas en displayMapping.columns, NORMALIZADAS con
// los defaults de compatibilidad: Visible ausente ⇒ true; IsPrimaryKey ausente ⇒
// false; Editable ausente ⇒ true, salvo PK (que fuerza Editable=false). Tolera
// definiciones sin la clave "columns" (devuelve nil) y JSONB malformado
// (devuelve las columnas que sí parseen). No falla nunca.
func ColumnsFromDisplayMapping(dm map[string]any) []ResolvedColumn {
	return ColumnsFromDisplayMappingWithPrincipal(dm, "")
}

// ColumnsFromDisplayMappingWithPrincipal es como ColumnsFromDisplayMapping pero
// resuelve el default de SourceTable: una columna sin sourceTable declarada toma
// `principal` (compatibilidad N=1, R16.2). Con principal == "" el SourceTable
// queda vacío y el resto del motor lo trata como principal, idéntico al
// comportamiento histórico (ColumnsFromDisplayMapping delega así sin cambiar
// llamadas existentes).
//
// El orden estable es (Order, Field, SourceTable). En N=1 SourceTable es
// CONSTANTE (la principal, o vacío), por lo que el tercer criterio nunca
// desempata y la salida es idéntica al histórico (Order, Field).
func ColumnsFromDisplayMappingWithPrincipal(dm map[string]any, principal string) []ResolvedColumn {
	if dm == nil {
		return nil
	}
	raw, ok := dm["columns"]
	if !ok {
		return nil
	}
	// Round-trip tolerante por JSON (mismo patrón que decodeConditions): se
	// serializa el valor opaco y se intenta deserializar a []DisplayColumn.
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var cols []DisplayColumn
	if err := json.Unmarshal(b, &cols); err != nil {
		return nil
	}

	out := make([]ResolvedColumn, 0, len(cols))
	for _, c := range cols {
		if c.Field == "" {
			continue
		}
		visible := true
		if c.Visible != nil {
			visible = *c.Visible
		}
		editable := true
		if c.Editable != nil {
			editable = *c.Editable
		}
		// Invariante PK ⇒ no editable (refuerzo en la capa de lectura).
		if c.IsPrimaryKey {
			editable = false
		}
		sourceTable := c.SourceTable
		if sourceTable == "" {
			sourceTable = principal
		}
		out = append(out, ResolvedColumn{
			Field:        c.Field,
			Label:        c.Label,
			Order:        c.Order,
			Format:       c.Format,
			Visible:      visible,
			IsPrimaryKey: c.IsPrimaryKey,
			Editable:     editable,
			SourceTable:  sourceTable,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		return out[i].SourceTable < out[j].SourceTable
	})
	return out
}

// PrimaryKeyColumns devuelve, en orden estable, los Field de las columnas con
// IsPrimaryKey=true.
func PrimaryKeyColumns(cols []ResolvedColumn) []string {
	keys := make([]string, 0)
	for _, c := range cols {
		if c.IsPrimaryKey {
			keys = append(keys, c.Field)
		}
	}
	sort.Strings(keys)
	return keys
}

// EditableColumns devuelve, en orden estable, los Field de las columnas con
// Editable=true y NO PK (definen el SET del UPDATE).
func EditableColumns(cols []ResolvedColumn) []string {
	editable := make([]string, 0)
	for _, c := range cols {
		if c.Editable && !c.IsPrimaryKey {
			editable = append(editable, c.Field)
		}
	}
	sort.Strings(editable)
	return editable
}

// ActionWriteColumns devuelve, en orden estable, los Field de las columnas que
// una acción de escritura DECLARA como objetivo (las que entran en el SET de un
// UPDATE). Impone el TECHO DE SEGURIDAD: solo se devuelven columnas EDITABLES y
// NO PK; una columna no editable, PK o inexistente NUNCA se devuelve aunque la
// acción la liste en sus InputFields (R3.1/R3.2). Reglas:
//
//   - Si la acción NO declara InputFields, devuelve TODAS las columnas editables
//     no-PK (compatibilidad: acción sin configuración = comportamiento de hoy,
//     todas las editables capturadas, R8.2).
//   - Si declara InputFields, devuelve la intersección de las columnas editables
//     no-PK con los Name de esos campos (las columnas objetivo que la acción
//     toca). La correspondencia es por nombre (FieldSpec.Name == columna.Field).
//
// El backend es la autoridad: lo que la UI ofrezca o deje de ofrecer no cambia
// este techo.
func ActionWriteColumns(cols []ResolvedColumn, act Action) []string {
	editable := EditableColumns(cols) // editables no-PK, orden estable
	if len(act.InputFields) == 0 {
		return editable
	}
	targets := make(map[string]struct{}, len(act.InputFields))
	for _, f := range act.InputFields {
		targets[f.Name] = struct{}{}
	}
	out := make([]string, 0, len(editable))
	for _, c := range editable {
		if _, ok := targets[c]; ok {
			out = append(out, c)
		}
	}
	return out
}

// ColumnsByTable agrupa las columnas por su SourceTable (ya resuelta). El orden
// dentro de cada tabla conserva el orden estable de `cols`.
func ColumnsByTable(cols []ResolvedColumn) map[string][]ResolvedColumn {
	out := make(map[string][]ResolvedColumn)
	for _, c := range cols {
		out[c.SourceTable] = append(out[c.SourceTable], c)
	}
	return out
}

// PrimaryKeyColumnsByTable devuelve, por tabla, sus columnas PK (Field) en orden
// estable alfabético.
func PrimaryKeyColumnsByTable(cols []ResolvedColumn) map[string][]string {
	out := make(map[string][]string)
	for _, c := range cols {
		if c.IsPrimaryKey {
			out[c.SourceTable] = append(out[c.SourceTable], c.Field)
		}
	}
	for t := range out {
		sort.Strings(out[t])
	}
	return out
}

// ActionWriteColumnsByTable empareja cada InputField OBJETIVO de la acción con
// la ResolvedColumn correcta por (sourceTable, field) y la coloca en el bucket
// de su tabla. Generaliza ActionWriteColumns a multi-tabla SIN delegar en ella
// (que empareja solo por nombre y con homónimas tomaría la primera columna).
// Impone el techo de seguridad por columna: solo entran columnas EDITABLES y
// NO-PK. Devuelve map[tabla][]field, cada lista en orden estable.
//
// Reglas de emparejamiento de cada InputField f:
//   - f.SourceTable != "": se busca la columna (f.SourceTable, f.Name).
//   - f.SourceTable == "": se busca la ÚNICA columna con Field == f.Name; si es
//     homónima (más de una), se ignora aquí (save-time ya rechazó esa definición).
//   - Si la columna no existe, no es Editable, o es PK ⇒ NO se incluye (techo).
//
// Si la acción NO declara InputFields (compatibilidad), devuelve TODAS las
// editables no-PK repartidas por su SourceTable (equivale al comportamiento N=1).
func ActionWriteColumnsByTable(cols []ResolvedColumn, act Action) map[string][]string {
	// Índice por (sourceTable, field) y conteo de homónimos por field.
	byKey := make(map[[2]string]ResolvedColumn, len(cols))
	countByField := make(map[string]int, len(cols))
	firstByField := make(map[string]ResolvedColumn, len(cols))
	for _, c := range cols {
		byKey[[2]string{c.SourceTable, c.Field}] = c
		countByField[c.Field]++
		if _, ok := firstByField[c.Field]; !ok {
			firstByField[c.Field] = c
		}
	}

	out := make(map[string][]string)
	appendTarget := func(c ResolvedColumn) {
		if !c.Editable || c.IsPrimaryKey {
			return // techo de seguridad: solo editables no-PK
		}
		out[c.SourceTable] = append(out[c.SourceTable], c.Field)
	}

	if len(act.InputFields) == 0 {
		for _, c := range cols {
			appendTarget(c)
		}
	} else {
		for _, f := range act.InputFields {
			if f.SourceTable != "" {
				if c, ok := byKey[[2]string{f.SourceTable, f.Name}]; ok {
					appendTarget(c)
				}
				continue
			}
			// Sin sourceTable: solo si el nombre es único.
			if countByField[f.Name] == 1 {
				appendTarget(firstByField[f.Name])
			}
			// Homónima sin desambiguar: se ignora (save-time la rechazó).
		}
	}

	for t := range out {
		sort.Strings(out[t])
	}
	return out
}

// EditFieldSpecs deriva los FieldSpec que ValidateInput debe usar para una
// acción edit/delete GENERADA DESDE COLUMNAS y COMBINADA con la configuración de
// la acción. Siempre emite un FieldSpec por columna PK (obligatorio,
// FieldTypeKeyPassthrough: solo se valida su presencia; aporta el valor del
// WHERE). Para edit, emite además un FieldSpec por cada columna OBJETIVO de la
// acción (ActionWriteColumns): si la acción declaró ese campo en InputFields, se
// REUSA tal cual (conserva Source/ConstValue/Options/DefaultValue/Required/Type,
// de modo que la validación respete constant/select/capturado); si la acción NO
// declara InputFields (compatibilidad), se emite uno capturado (Source input,
// Type inferido del formato) por cada columna editable. Para delete solo se
// derivan las columnas PK (el DELETE no tiene SET).
//
// MULTI-TABLA (spec.IsMultiTable()): el Name de cada FieldSpec PK es el alias
// "<T>__<k>" (el MISMO del SELECT, del frontend y del motor de escritura), de
// modo que ValidateInput exija exactamente la clave que trae el input. Las PK
// emitidas son la unión de las de las tablas AFECTADAS (delete: las de
// DeleteOrder(DeleteTopTable); edit: las de cada tabla con columnas objetivo).
// En N=1 el Name es el nombre PELADO, idéntico a hoy. Si el árbol falla en
// ejecución (definición corrupta), cae a la UNION-MÁXIMA de PK de TODAS las
// tablas declaradas (≥ al real), conservando el retorno []FieldSpec sin error.
func EditFieldSpecs(cols []ResolvedColumn, act Action, spec MultiTableSpec) []FieldSpec {
	multi := spec.IsMultiTable()
	pkName := func(table, field string) string {
		if multi {
			return table + "__" + field
		}
		return field
	}

	specs := make([]FieldSpec, 0, len(cols))
	emitPK := func(table, field string) {
		// La PK se valida SOLO por presencia: su valor llega verbatim de la fila
		// (float64 para una PK numérica, etc.), con el sentinel
		// FieldTypeKeyPassthrough para que checkType no valide su forma (R7.4).
		name := pkName(table, field)
		specs = append(specs, FieldSpec{
			Name:     name,
			Type:     FieldTypeKeyPassthrough,
			Required: true,
			Source:   FieldSourceRow,
			RowField: name,
		})
	}

	if !multi {
		for _, c := range cols {
			if c.IsPrimaryKey {
				emitPK(c.SourceTable, c.Field)
			}
		}
	} else {
		pkByTable := PrimaryKeyColumnsByTable(cols)
		affected := editFieldAffectedTables(cols, act, spec, pkByTable)
		tables := make([]string, 0, len(affected))
		for t := range affected {
			tables = append(tables, t)
		}
		sort.Strings(tables)
		for _, t := range tables {
			for _, k := range pkByTable[t] {
				emitPK(t, k)
			}
		}
	}

	if act.Kind != ActionKindEdit {
		return specs // delete: solo PK
	}

	// Columnas objetivo de la acción (techo editable ya aplicado). En multi-tabla
	// se reparten por tabla, pero la identidad del FieldSpec objetivo conserva su
	// nombre de columna (con SourceTable cuando es homónima).
	var targetSet map[[2]string]struct{}
	if multi {
		byTable := ActionWriteColumnsByTable(cols, act)
		targetSet = make(map[[2]string]struct{})
		for t, fields := range byTable {
			for _, f := range fields {
				targetSet[[2]string{t, f}] = struct{}{}
			}
		}
	} else {
		targets := ActionWriteColumns(cols, act)
		targetSet = make(map[[2]string]struct{}, len(targets))
		for _, t := range targets {
			targetSet[[2]string{"", t}] = struct{}{}
		}
	}

	// Indexa los InputFields declarados por la acción por (sourceTable, nombre).
	declared := make(map[[2]string]FieldSpec, len(act.InputFields))
	for _, f := range act.InputFields {
		declared[[2]string{f.SourceTable, f.Name}] = f
	}

	for _, c := range cols {
		if c.IsPrimaryKey {
			continue
		}
		key := [2]string{c.SourceTable, c.Field}
		if !multi {
			key = [2]string{"", c.Field}
		}
		if _, ok := targetSet[key]; !ok {
			continue
		}
		// Busca el InputField declarado por (sourceTable,nombre) o por nombre pelado.
		if f, ok := declared[[2]string{c.SourceTable, c.Field}]; ok {
			specs = append(specs, f)
			continue
		}
		if f, ok := declared[[2]string{"", c.Field}]; ok {
			specs = append(specs, f)
			continue
		}
		specs = append(specs, FieldSpec{
			Name:     c.Field,
			Type:     typeFromFormat(c.Format),
			Required: false,
			Source:   FieldSourceInput,
		})
	}
	return specs
}

// editFieldAffectedTables devuelve el conjunto de tablas cuyas PK debe exigir
// EditFieldSpecs en multi-tabla. Para delete: las tablas de
// DeleteOrder(DeleteTopTable); para edit: las tablas con columnas objetivo. Ante
// un árbol inválido en ejecución, cae a la union-máxima (todas las tablas
// declaradas) para NUNCA exigir menos PK que las reales.
func editFieldAffectedTables(cols []ResolvedColumn, act Action, spec MultiTableSpec, pkByTable map[string][]string) map[string]struct{} {
	affected := make(map[string]struct{})
	allDeclared := func() map[string]struct{} {
		out := make(map[string]struct{})
		for _, t := range spec.Tables() {
			if len(pkByTable[t]) > 0 {
				out[t] = struct{}{}
			}
		}
		return out
	}

	switch act.Kind {
	case ActionKindDelete:
		tree, err := BuildRelationTree(spec)
		if err != nil {
			return allDeclared()
		}
		top := act.DeleteTopTable
		if top == "" {
			top = spec.PrincipalTable
		}
		order, err := tree.DeleteOrder(top)
		if err != nil {
			return allDeclared()
		}
		for _, t := range order {
			affected[t] = struct{}{}
		}
	case ActionKindEdit:
		byTable := ActionWriteColumnsByTable(cols, act)
		for t, fields := range byTable {
			if len(fields) > 0 {
				affected[t] = struct{}{}
			}
		}
	default:
		// Otros tipos: no se exige PK por tabla.
	}
	return affected
}

// typeFromFormat traduce el formato de presentación de una columna al tipo de
// campo de entrada que ValidateInput entiende. Solo las fechas tienen un mapeo
// específico; el resto se trata como texto.
func typeFromFormat(format string) string {
	switch format {
	case "date", "datetime":
		return "date"
	default:
		return "string"
	}
}

// hasWriteActionEditOrDelete indica si alguna acción es de tipo editar o
// eliminar (las que requieren una columna llave primaria para el WHERE).
func hasWriteActionEditOrDelete(actions []Action) bool {
	for _, a := range actions {
		if a.Kind == ActionKindEdit || a.Kind == ActionKindDelete {
			return true
		}
	}
	return false
}
