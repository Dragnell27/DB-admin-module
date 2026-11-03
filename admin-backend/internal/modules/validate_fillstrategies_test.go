package modules

import (
	"errors"
	"testing"
)

// validFillStrategyDef arma una definición multi-tabla VÁLIDA cuyo INSERT a
// gt_valores_hijo usa las cinco estrategias de llenado (caso real "activar medio
// de pago para un comercio"). Se muta en cada test negativo.
//
// Capturas: __capture_comercio (cl_comercios_sapp, no escribe) y valor_lista
// (gt_valores, escribe + deriva list_numero_h). Derivaciones: list_numero_h
// (derive-from-option), padr_numero y depe_numero (derive-via-fk que comparten
// el mismo salto a gt_valores_padre). Constantes: usrio_numero, eliminado.
func validFillStrategyDef() *Definition {
	return &Definition{
		Slug:       "activar-medio-pago",
		Name:       "Activar medio de pago",
		SourceType: SourcePostgres,
		FetchMapping: map[string]any{
			"table": "gt_valores_hijo",
			"foreignTables": []any{
				map[string]any{"table": "gt_valores"},
			},
			"relationships": []any{
				map[string]any{
					"childTable":  "gt_valores_hijo",
					"parentTable": "gt_valores",
					"on":          []any{map[string]any{"child": "list_numero_h", "parent": "list_numero"}},
					"joinType":    "inner",
				},
			},
		},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{"field": "hijo_numero", "isPrimaryKey": true, "sourceTable": "gt_valores_hijo", "order": 1},
				map[string]any{"field": "valor_lista", "editable": true, "sourceTable": "gt_valores_hijo", "order": 2},
				map[string]any{"field": "list_numero_h", "editable": true, "sourceTable": "gt_valores_hijo", "order": 3},
				map[string]any{"field": "padr_numero", "editable": true, "sourceTable": "gt_valores_hijo", "order": 4},
				map[string]any{"field": "depe_numero", "editable": true, "sourceTable": "gt_valores_hijo", "order": 5},
				map[string]any{"field": "usrio_numero", "editable": true, "sourceTable": "gt_valores_hijo", "order": 6},
				map[string]any{"field": "eliminado", "editable": true, "sourceTable": "gt_valores_hijo", "order": 7},
				map[string]any{"field": "list_numero", "sourceTable": "gt_valores", "order": 8},
			},
		},
		DataSource: DataSource{
			SourceType: SourcePostgres,
			PGWhitelist: map[string][]string{
				"gt_valores_hijo":   {"hijo_numero", "valor_lista", "list_numero_h", "padr_numero", "depe_numero", "usrio_numero", "eliminado"},
				"gt_valores":        {"valo_numero", "list_numero", "codigo", "valor_es"},
				"cl_comercios_sapp": {"come_numero", "nombre"},
				"gt_valores_padre":  {"padr_numero", "depe_numero", "valor_objeto"},
			},
			ConnectionTemplateID: "tpl-1",
		},
		Actions: []Action{
			{
				ActionKey:   "activar",
				Kind:        ActionKindCreate,
				SQLOp:       "INSERT",
				TargetTable: "gt_valores_hijo",
				InputFields: []FieldSpec{
					{Name: "__capture_comercio", Source: FieldSourceFillStrategy, Required: true, Strategy: &FillStrategy{
						Kind:       StrategyCaptureFromCatalog,
						CaptureKey: "comercio",
						Catalog:    &DynamicSource{Table: "cl_comercios_sapp", IDColumn: "come_numero", LabelColumn: "nombre"},
					}},
					{Name: "valor_lista", Source: FieldSourceFillStrategy, Required: true, Strategy: &FillStrategy{
						Kind:        StrategyCaptureFromCatalog,
						CaptureKey:  "medio_pago",
						StoreColumn: "codigo",
						Catalog:     &DynamicSource{Table: "gt_valores", IDColumn: "valo_numero", LabelColumn: "valor_es"},
					}},
					{Name: "list_numero_h", Source: FieldSourceFillStrategy, Strategy: &FillStrategy{
						Kind:             StrategyDeriveFromOption,
						SourceCaptureKey: "medio_pago",
						Attribute:        "list_numero",
					}},
					{Name: "padr_numero", Source: FieldSourceFillStrategy, Strategy: &FillStrategy{
						Kind: StrategyDeriveViaFK,
						Jump: &FkJump{RelatedTable: "gt_valores_padre", RelatedColumn: "valor_objeto", LocalCaptureKey: "comercio", LocalAttribute: "come_numero"},
						Mapping: []AttributeMap{{Attribute: "padr_numero", TargetColumn: "padr_numero"}},
					}},
					{Name: "depe_numero", Source: FieldSourceFillStrategy, Strategy: &FillStrategy{
						Kind: StrategyDeriveViaFK,
						Jump: &FkJump{RelatedTable: "gt_valores_padre", RelatedColumn: "valor_objeto", LocalCaptureKey: "comercio", LocalAttribute: "come_numero"},
						Mapping: []AttributeMap{{Attribute: "depe_numero", TargetColumn: "depe_numero"}},
					}},
					{Name: "usrio_numero", Source: FieldSourceConstant, ConstValue: "1"},
					{Name: "eliminado", Source: FieldSourceConstant, ConstValue: "-1"},
				},
			},
		},
	}
}

func TestValidateFillStrategies_ValidDef(t *testing.T) {
	if err := validateDefinition(validFillStrategyDef()); err != nil {
		t.Fatalf("la definición con estrategias válida no debe fallar: %v", err)
	}
}

// Regla 1: estrategias sin TargetTable ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_NoTargetTable(t *testing.T) {
	def := validFillStrategyDef()
	def.Actions[0].TargetTable = ""
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por falta de tabla objetivo: %v", err)
	}
}

// Regla 2: derive-via-fk con Mapping != 1 entrada ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_DeriveViaFKMappingLen(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "padr_numero" {
			f.Strategy.Mapping = nil // 0 entradas
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por Mapping inválido: %v", err)
	}
}

// Regla 2: derive-via-fk con TargetColumn != Name ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_DeriveViaFKTargetColumnMismatch(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "padr_numero" {
			f.Strategy.Mapping = []AttributeMap{{Attribute: "padr_numero", TargetColumn: "otra_col"}}
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por targetColumn != name: %v", err)
	}
}

// Regla 2: Name destino de una derivación fuera de la whitelist de la tabla
// objetivo ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_NameOutOfTargetWhitelist(t *testing.T) {
	def := validFillStrategyDef()
	// Quita list_numero_h de la whitelist de la tabla objetivo.
	def.DataSource.PGWhitelist["gt_valores_hijo"] = []string{"hijo_numero", "valor_lista", "padr_numero", "depe_numero", "usrio_numero", "eliminado"}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por columna destino fuera de whitelist: %v", err)
	}
}

// Regla 3: un __capture_ que colisiona con columna de la tabla objetivo ⇒
// ErrMultiTableValidation.
func TestValidateFillStrategies_CapturePrefixCollidesWithColumn(t *testing.T) {
	def := validFillStrategyDef()
	// Añade __capture_x como columna de la tabla objetivo (colisión artificial).
	def.DataSource.PGWhitelist["gt_valores_hijo"] = append(def.DataSource.PGWhitelist["gt_valores_hijo"], "__capture_x")
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "__capture_comercio" {
			f.Name = "__capture_x"
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por __capture_ que colisiona con columna: %v", err)
	}
}

// Regla 4: CaptureKey duplicado ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_DuplicateCaptureKey(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "valor_lista" {
			f.Strategy.CaptureKey = "comercio" // duplica el de __capture_comercio
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por captureKey duplicado: %v", err)
	}
}

// Regla 4: SourceCaptureKey sin resolver ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_SourceCaptureKeyUnresolved(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "list_numero_h" {
			f.Strategy.SourceCaptureKey = "inexistente"
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por sourceCaptureKey sin resolver: %v", err)
	}
}

// Regla 4: Jump.LocalCaptureKey sin resolver ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_LocalCaptureKeyUnresolved(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "padr_numero" {
			f.Strategy.Jump.LocalCaptureKey = "inexistente"
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por localCaptureKey sin resolver: %v", err)
	}
}

// Regla 5: catálogo/salto fuera de la whitelist ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_CatalogOutOfWhitelist(t *testing.T) {
	def := validFillStrategyDef()
	// Quita la tabla del salto de la whitelist.
	delete(def.DataSource.PGWhitelist, "gt_valores_padre")
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por tabla de salto fuera de whitelist: %v", err)
	}
}

// Regla 5 (INV-7): Jump.LocalAttribute fuera de la whitelist del catálogo local
// ⇒ ErrMultiTableValidation.
func TestValidateFillStrategies_LocalAttributeOutOfCatalogWhitelist(t *testing.T) {
	def := validFillStrategyDef()
	for i := range def.Actions[0].InputFields {
		f := &def.Actions[0].InputFields[i]
		if f.Name == "padr_numero" {
			f.Strategy.Jump.LocalAttribute = "no_existe_en_comercios"
		}
	}
	err := validateFillStrategies(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba rechazo por localAttribute fuera de whitelist del catálogo local: %v", err)
	}
}
