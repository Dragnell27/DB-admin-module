package modules

import (
	"errors"
	"testing"
)

// validMultiDef arma una definición multi-tabla VÁLIDA (pagos ← clientes, pagos
// hijo FK de clientes) con una acción edit y una delete, para mutarla en cada
// test negativo.
func validMultiDef() *Definition {
	return &Definition{
		Slug:       "pagos-clientes",
		Name:       "Pagos con clientes",
		SourceType: SourcePostgres,
		FetchMapping: map[string]any{
			"table": "pagos",
			"foreignTables": []any{
				map[string]any{"table": "clientes"},
			},
			"relationships": []any{
				map[string]any{
					"childTable":  "pagos",
					"parentTable": "clientes",
					"on":          []any{map[string]any{"child": "cliente_id", "parent": "id"}},
					"joinType":    "inner",
				},
			},
		},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "order": 1},
				map[string]any{"field": "cliente_id", "editable": true, "sourceTable": "pagos", "order": 2},
				map[string]any{"field": "monto", "editable": true, "sourceTable": "pagos", "order": 3},
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes", "order": 4},
				map[string]any{"field": "nombre", "editable": true, "sourceTable": "clientes", "order": 5},
			},
		},
		DataSource: DataSource{
			SourceType: SourcePostgres,
			PGWhitelist: map[string][]string{
				"pagos":    {"id", "cliente_id", "monto"},
				"clientes": {"id", "nombre"},
			},
			ConnectionTemplateID: "tpl-1",
		},
		Actions: []Action{
			{ActionKey: "edit", Kind: ActionKindEdit, SQLOp: "UPDATE", InputFields: []FieldSpec{
				{Name: "monto", SourceTable: "pagos", Source: FieldSourceInput},
				{Name: "nombre", SourceTable: "clientes", Source: FieldSourceInput},
			}},
			{ActionKey: "delete", Kind: ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"},
		},
	}
}

func TestValidateMultiTable_ValidDef(t *testing.T) {
	if err := validateMultiTable(validMultiDef()); err != nil {
		t.Fatalf("definición válida no debe fallar: %v", err)
	}
}

func TestValidateMultiTable_N1NoOp(t *testing.T) {
	def := &Definition{
		FetchMapping:   map[string]any{"table": "planes"},
		DisplayMapping: map[string]any{"columns": []any{map[string]any{"field": "id", "isPrimaryKey": true}}},
		DataSource:     DataSource{PGWhitelist: map[string][]string{"planes": {"id"}}},
	}
	if err := validateMultiTable(def); err != nil {
		t.Fatalf("N=1 debe ser no-op: %v", err)
	}
}

// Test 5: tabla escribible sin PK y grafo cíclico ⇒ ErrMultiTableValidation.
func TestValidateMultiTable_WritableTableNoPK(t *testing.T) {
	def := validMultiDef()
	// Quita la PK de clientes; el edit toca clientes.nombre ⇒ debe exigir PK.
	def.DisplayMapping = map[string]any{
		"columns": []any{
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "order": 1},
			map[string]any{"field": "cliente_id", "editable": true, "sourceTable": "pagos", "order": 2},
			map[string]any{"field": "monto", "editable": true, "sourceTable": "pagos", "order": 3},
			map[string]any{"field": "id", "sourceTable": "clientes", "order": 4}, // ya no es PK
			map[string]any{"field": "nombre", "editable": true, "sourceTable": "clientes", "order": 5},
		},
	}
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por tabla sin PK, got %v", err)
	}
}

func TestValidateMultiTable_CyclicGraph(t *testing.T) {
	def := validMultiDef()
	def.FetchMapping = map[string]any{
		"table":         "pagos",
		"foreignTables": []any{map[string]any{"table": "clientes"}},
		"relationships": []any{
			map[string]any{"childTable": "pagos", "parentTable": "clientes", "on": []any{map[string]any{"child": "cliente_id", "parent": "id"}}, "joinType": "inner"},
			map[string]any{"childTable": "clientes", "parentTable": "pagos", "on": []any{map[string]any{"child": "id", "parent": "id"}}, "joinType": "inner"},
		},
	}
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por ciclo FK, got %v", err)
	}
}

// Test 4: una table.column fuera de la whitelist en cualquier tabla.
func TestValidateMultiTable_OutOfWhitelist(t *testing.T) {
	t.Run("columna de visualización fuera de whitelist (foránea)", func(t *testing.T) {
		def := validMultiDef()
		def.DataSource.PGWhitelist["clientes"] = []string{"id"} // falta "nombre"
		err := validateMultiTable(def)
		if err == nil || !errors.Is(err, ErrMultiTableValidation) {
			t.Fatalf("se esperaba rechazo por columna fuera de whitelist: %v", err)
		}
	})

	t.Run("columna de ON fuera de whitelist (principal)", func(t *testing.T) {
		def := validMultiDef()
		def.DataSource.PGWhitelist["pagos"] = []string{"id", "monto"} // falta "cliente_id"
		err := validateMultiTable(def)
		if err == nil || !errors.Is(err, ErrMultiTableValidation) {
			t.Fatalf("se esperaba rechazo por columna de ON fuera de whitelist: %v", err)
		}
	})

	t.Run("tabla referenciada sin entrada en whitelist", func(t *testing.T) {
		def := validMultiDef()
		delete(def.DataSource.PGWhitelist, "clientes")
		err := validateMultiTable(def)
		if err == nil || !errors.Is(err, ErrMultiTableValidation) {
			t.Fatalf("se esperaba rechazo por tabla sin whitelist: %v", err)
		}
	})
}

func TestValidateMultiTable_AmbiguousField(t *testing.T) {
	def := validMultiDef()
	// Añade una columna homónima "monto" también en clientes.
	def.DataSource.PGWhitelist["clientes"] = []string{"id", "nombre", "monto"}
	def.DisplayMapping = map[string]any{
		"columns": []any{
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "order": 1},
			map[string]any{"field": "cliente_id", "editable": true, "sourceTable": "pagos", "order": 2},
			map[string]any{"field": "monto", "editable": true, "sourceTable": "pagos", "order": 3},
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes", "order": 4},
			map[string]any{"field": "nombre", "editable": true, "sourceTable": "clientes", "order": 5},
			map[string]any{"field": "monto", "editable": true, "sourceTable": "clientes", "order": 6},
		},
	}
	// El edit nombra "monto" SIN sourceTable ⇒ ambiguo.
	def.Actions[0].InputFields = []FieldSpec{{Name: "monto", Source: FieldSourceInput}}
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por campo homónimo ambiguo: %v", err)
	}
}

func TestValidateMultiTable_BadJoinType(t *testing.T) {
	def := validMultiDef()
	def.FetchMapping = map[string]any{
		"table":         "pagos",
		"foreignTables": []any{map[string]any{"table": "clientes"}},
		"relationships": []any{
			map[string]any{"childTable": "pagos", "parentTable": "clientes", "on": []any{map[string]any{"child": "cliente_id", "parent": "id"}}, "joinType": "full"},
		},
	}
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por joinType inválido: %v", err)
	}
}

func TestValidateMultiTable_BadTargetAndTop(t *testing.T) {
	t.Run("delete top fuera del módulo", func(t *testing.T) {
		def := validMultiDef()
		def.Actions[1].DeleteTopTable = "otra"
		err := validateMultiTable(def)
		if err == nil || !errors.Is(err, ErrMultiTableValidation) {
			t.Fatalf("se esperaba rechazo por tope inválido: %v", err)
		}
	})

	t.Run("create target fuera del módulo", func(t *testing.T) {
		def := validMultiDef()
		def.Actions = append(def.Actions, Action{ActionKey: "create", Kind: ActionKindCreate, SQLOp: "INSERT", TargetTable: "otra"})
		err := validateMultiTable(def)
		if err == nil || !errors.Is(err, ErrMultiTableValidation) {
			t.Fatalf("se esperaba rechazo por target inválido: %v", err)
		}
	})
}

func TestValidateMultiTable_AliasTooLong(t *testing.T) {
	def := validMultiDef()
	longCol := ""
	for i := 0; i < 70; i++ {
		longCol += "x"
	}
	def.DataSource.PGWhitelist["pagos"] = append(def.DataSource.PGWhitelist["pagos"], longCol)
	def.DisplayMapping = map[string]any{
		"columns": []any{
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "order": 1},
			map[string]any{"field": "cliente_id", "editable": true, "sourceTable": "pagos", "order": 2},
			map[string]any{"field": longCol, "sourceTable": "pagos", "order": 3},
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes", "order": 4},
			map[string]any{"field": "nombre", "editable": true, "sourceTable": "clientes", "order": 5},
		},
	}
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por alias > 63 bytes: %v", err)
	}
}

// multiDefWithOnConstant añade a validMultiDef una relación con una constante del
// ON (clientes.nombre = ...) y la lista blanca necesaria.
func multiDefWithOnConstant(onConstants []any) *Definition {
	def := validMultiDef()
	def.FetchMapping["relationships"] = []any{
		map[string]any{
			"childTable":  "pagos",
			"parentTable": "clientes",
			"on":          []any{map[string]any{"child": "cliente_id", "parent": "id"}},
			"onConstants": onConstants,
			"joinType":    "inner",
		},
	}
	return def
}

// TestValidateMultiTable_OnConstantValid acepta una constante del ON cuya tabla
// pertenece a la relación y cuya columna está en la lista blanca.
func TestValidateMultiTable_OnConstantValid(t *testing.T) {
	def := multiDefWithOnConstant([]any{
		map[string]any{"table": "clientes", "column": "nombre", "value": "1"},
	})
	if err := validateMultiTable(def); err != nil {
		t.Fatalf("constante válida no debe fallar: %v", err)
	}
}

// TestValidateMultiTable_OnConstantForeignTable rechaza con E422 una constante
// cuya tabla es ajena a la relación.
func TestValidateMultiTable_OnConstantForeignTable(t *testing.T) {
	def := multiDefWithOnConstant([]any{
		map[string]any{"table": "otra", "column": "nombre", "value": "1"},
	})
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por tabla ajena en la constante: %v", err)
	}
}

// TestValidateMultiTable_OnConstantOutOfWhitelist rechaza con E422 una constante
// cuya columna no está en la lista blanca de su tabla.
func TestValidateMultiTable_OnConstantOutOfWhitelist(t *testing.T) {
	def := multiDefWithOnConstant([]any{
		map[string]any{"table": "clientes", "column": "estado", "value": "1"},
	})
	err := validateMultiTable(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por columna fuera de whitelist: %v", err)
	}
}
