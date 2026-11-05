package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"admin-backend/internal/auth"
	"admin-backend/internal/connections"
	"admin-backend/internal/secrets"
)

// Repository define la persistencia de las definiciones de módulo (Req. 1, 2).
// Las credenciales de origen se cifran al persistir y nunca se devuelven en los
// modelos de dominio públicos.
type Repository interface {
	// List devuelve todas las definiciones (con origen sin secretos y acciones).
	List(ctx context.Context) ([]Definition, error)
	// FindBySlug busca una definición por slug. Devuelve ErrNotFound si no existe.
	FindBySlug(ctx context.Context, slug string) (*Definition, error)
	// Create inserta una definición nueva (módulo + origen + acciones) en una
	// transacción, cifrando las credenciales. ErrSlugTaken si el slug ya existe.
	Create(ctx context.Context, def *Definition, sec Secrets) error
	// Update reemplaza por completo la definición (origen + acciones). Si algún
	// secreto viene vacío se conserva el cifrado previo (edición sin cambios).
	Update(ctx context.Context, def *Definition, sec Secrets) error
	// Delete elimina la definición (y por cascade su origen y acciones).
	Delete(ctx context.Context, slug string) error
}

// SecretBundle son las credenciales DESCIFRADAS de un origen, para uso exclusivo
// del motor de ejecución en memoria. NUNCA se serializa hacia el frontend.
type SecretBundle = Secrets

// gormRepository implementa Repository sobre GORM, cifrando los valores VARIABLE
// con un secrets.Sealer y ensamblando el DSN efectivo desde la plantilla.
type gormRepository struct {
	db       *gorm.DB
	sealer   *secrets.Sealer
	connRepo connections.Repository
}

// NewRepository crea un Repository sobre la conexión GORM dada, el sellador de
// secretos (internal/secrets) para cifrar/descifrar los valores VARIABLE en
// reposo y el repositorio de plantillas de conexión (para validar la FK,
// resolver las claves VARIABLE y ensamblar el DSN efectivo). La dirección de
// importación es modules -> connections (nunca al revés).
func NewRepository(db *gorm.DB, sealer *secrets.Sealer, connRepo connections.Repository) Repository {
	return &gormRepository{db: db, sealer: sealer, connRepo: connRepo}
}

// List devuelve todas las definiciones con su origen (sin secretos) y acciones.
func (r *gormRepository) List(ctx context.Context) ([]Definition, error) {
	var defRows []definitionRow
	if err := r.db.WithContext(ctx).Order("slug").Find(&defRows).Error; err != nil {
		return nil, fmt.Errorf("modules: error listando módulos: %w", err)
	}
	out := make([]Definition, 0, len(defRows))
	for _, dr := range defRows {
		def, err := r.assemble(ctx, dr)
		if err != nil {
			return nil, err
		}
		out = append(out, *def)
	}
	return out, nil
}

// FindBySlug recupera una definición completa por slug.
func (r *gormRepository) FindBySlug(ctx context.Context, slug string) (*Definition, error) {
	var dr definitionRow
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&dr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("modules: error buscando módulo por slug: %w", err)
	}
	return r.assemble(ctx, dr)
}

// assemble completa el modelo de dominio de una definición cargando su origen
// (sin exponer secretos) y sus acciones.
func (r *gormRepository) assemble(ctx context.Context, dr definitionRow) (*Definition, error) {
	def := &Definition{
		ID:           dr.ID,
		Slug:         dr.Slug,
		Name:         dr.Name,
		SourceType:   dr.SourceType,
		CountryScope: dr.CountryScope,
		MenuPath:     dr.MenuPath,
		CreatedAt:    dr.CreatedAt,
		UpdatedAt:    dr.UpdatedAt,
	}
	def.FetchMapping = decodeJSONObject(dr.FetchMapping)
	def.DisplayMapping = decodeJSONObject(dr.DisplayMapping)
	def.ListParams = decodeJSONObject(dr.ListParams)
	def.ListConditions = decodeConditions(dr.ListConditions)

	var ds dataSourceRow
	err := r.db.WithContext(ctx).Where("module_id = ?", dr.ID).First(&ds).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("modules: error cargando origen del módulo: %w", err)
	}
	if err == nil {
		ctid := ""
		if ds.ConnectionTemplateID != nil {
			ctid = *ds.ConnectionTemplateID
		}
		def.DataSource = DataSource{
			SourceType:           ds.SourceType,
			PGWhitelist:          decodeWhitelist(ds.PGWhitelist),
			ConnectionTemplateID: ctid,
			HasSecrets:           len(ds.SecretsEnc) > 0,
		}
	}

	var actRows []actionRow
	if err := r.db.WithContext(ctx).Where("module_id = ?", dr.ID).Order("action_key").Find(&actRows).Error; err != nil {
		return nil, fmt.Errorf("modules: error cargando acciones del módulo: %w", err)
	}
	for _, ar := range actRows {
		def.Actions = append(def.Actions, toActionDomain(ar))
	}
	return def, nil
}

// RevealSecrets descifra y devuelve las credenciales de un origen para uso del
// motor de ejecución EN MEMORIA. No forma parte de la interfaz pública Repository
// para dejar claro que es un camino privilegiado; se expone como método del tipo
// concreto y se accede vía un type assertion controlado desde el motor.
func (r *gormRepository) RevealSecrets(ctx context.Context, slug string) (SecretBundle, error) {
	var dr definitionRow
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&dr).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SecretBundle{}, ErrNotFound
		}
		return SecretBundle{}, fmt.Errorf("modules: error buscando módulo: %w", err)
	}
	var ds dataSourceRow
	if err := r.db.WithContext(ctx).Where("module_id = ?", dr.ID).First(&ds).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SecretBundle{}, nil
		}
		return SecretBundle{}, fmt.Errorf("modules: error cargando origen: %w", err)
	}
	// Un módulo sin plantilla (no migrado) se trata como "sin conexión": mismo
	// camino que el antiguo DSN vacío (el executor devuelve E500 "sin cadena").
	if ds.ConnectionTemplateID == nil || *ds.ConnectionTemplateID == "" {
		return SecretBundle{}, nil
	}

	// Estructura + valores FIJOS de la plantilla (descifrados en memoria).
	fields, fixed, err := r.connRepo.RevealFixedValues(ctx, *ds.ConnectionTemplateID)
	if err != nil {
		// Error genérico sin filtrar clave ni valores (Req. 4.5).
		return SecretBundle{}, fmt.Errorf("modules: no se pudo preparar la conexión del módulo %q", slug)
	}

	// Valores VARIABLE del módulo (bundle vacío `{}` es VÁLIDO: sin variables).
	variable := map[string]string{}
	if len(ds.SecretsEnc) > 0 {
		plain, oerr := r.sealer.Open(ds.SecretsEnc)
		if oerr != nil {
			return SecretBundle{}, fmt.Errorf("modules: no se pudieron descifrar las credenciales del módulo %q", slug)
		}
		var bundle Secrets
		if jerr := json.Unmarshal(plain, &bundle); jerr != nil {
			return SecretBundle{}, fmt.Errorf("modules: credenciales descifradas con formato inválido")
		}
		if bundle.VariableValues != nil {
			variable = bundle.VariableValues
		}
	}

	dsn, err := connections.AssembleDSN(fields, fixed, variable)
	if err != nil {
		// Detalle NO sensible (qué componente falta), nunca el valor.
		return SecretBundle{}, fmt.Errorf("modules: no se pudo formar la conexión del módulo %q: %w", slug, err)
	}
	return SecretBundle{DSN: dsn}, nil
}

// Create inserta la definición, su origen (con credenciales cifradas) y sus
// acciones en una transacción. Valida slug kebab-case y unicidad.
func (r *gormRepository) Create(ctx context.Context, def *Definition, sec Secrets) error {
	if def == nil {
		return fmt.Errorf("modules: la definición a crear no puede ser nil")
	}
	if err := validateDefinition(def); err != nil {
		return err
	}
	// Invariante de escritura: la plantilla es el ÚNICO mecanismo de conexión
	// (Req. 7.1). Debe estar presente y existir.
	if err := r.validateTemplateRef(ctx, def.DataSource.ConnectionTemplateID); err != nil {
		return err
	}

	sealed, err := r.sealSecrets(sec)
	if err != nil {
		return err
	}

	templateID := def.DataSource.ConnectionTemplateID

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&definitionRow{}).Where("slug = ?", def.Slug).Count(&count).Error; err != nil {
			return fmt.Errorf("modules: error verificando slug: %w", err)
		}
		if count > 0 {
			return ErrSlugTaken
		}

		dr := definitionRow{
			ID:             def.ID,
			Slug:           def.Slug,
			Name:           strings.TrimSpace(def.Name),
			SourceType:     def.SourceType,
			CountryScope:   defaultCountryScope(def.CountryScope),
			MenuPath:       strings.TrimSpace(def.MenuPath),
			FetchMapping:   encodeJSONObject(def.FetchMapping),
			DisplayMapping: encodeJSONObject(def.DisplayMapping),
			ListParams:     encodeJSONObject(def.ListParams),
			ListConditions: encodeConditions(def.ListConditions),
		}
		if err := tx.Create(&dr).Error; err != nil {
			return fmt.Errorf("modules: error creando módulo: %w", err)
		}
		def.ID = dr.ID

		// Un módulo nuevo nunca tiene legado (secretsEncLegacy = nil).
		if err := insertDataSource(tx, dr.ID, def, sealed, ptrOrNil(templateID), nil); err != nil {
			return err
		}
		if err := insertActions(tx, dr.ID, def.Actions); err != nil {
			return err
		}
		return nil
	})
}

// Update reemplaza totalmente origen y acciones del módulo identificado por su
// slug. Secretos vacíos conservan el cifrado previo (edición "sin cambios").
func (r *gormRepository) Update(ctx context.Context, def *Definition, sec Secrets) error {
	if def == nil {
		return fmt.Errorf("modules: la definición a actualizar no puede ser nil")
	}
	if err := validateDefinition(def); err != nil {
		return err
	}
	// Invariante de escritura: la plantilla es el ÚNICO mecanismo de conexión.
	if err := r.validateTemplateRef(ctx, def.DataSource.ConnectionTemplateID); err != nil {
		return err
	}

	// Claves VARIABLE de la plantilla referenciada (para el merge por clave).
	variableKeys, err := r.connRepo.VariableKeys(ctx, def.DataSource.ConnectionTemplateID)
	if err != nil {
		return fmt.Errorf("modules: no se pudieron resolver las claves variables de la plantilla: %w", err)
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dr definitionRow
		if err := tx.Where("slug = ?", def.Slug).First(&dr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("modules: error buscando módulo a actualizar: %w", err)
		}

		// Actualiza metadatos y mapeos.
		updates := map[string]any{
			"name":            strings.TrimSpace(def.Name),
			"source_type":     def.SourceType,
			"country_scope":   defaultCountryScope(def.CountryScope),
			"menu_path":       strings.TrimSpace(def.MenuPath),
			"fetch_mapping":   encodeJSONObject(def.FetchMapping),
			"display_mapping": encodeJSONObject(def.DisplayMapping),
			"list_params":     encodeJSONObject(def.ListParams),
			"list_conditions": encodeConditions(def.ListConditions),
		}
		if err := tx.Model(&definitionRow{}).Where("id = ?", dr.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("modules: error actualizando módulo: %w", err)
		}
		def.ID = dr.ID

		// Carga SIEMPRE la fila previa del origen: el merge por clave de los
		// valores VARIABLE y el arrastre de secrets_enc_legacy lo requieren.
		var prev dataSourceRow
		var hadPrev bool
		if perr := tx.Where("module_id = ?", dr.ID).First(&prev).Error; perr == nil {
			hadPrev = true
		} else if !errors.Is(perr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("modules: error cargando origen previo: %w", perr)
		}

		// Merge por clave de los valores VARIABLE contra el bundle previo
		// descifrado (vacío/ausente = conservar; presente = reemplazar; clave
		// fuera de variableKeys = descartar).
		sealed, err := mergeVariableValues(r.sealer, prev.SecretsEnc, sec, variableKeys)
		if err != nil {
			return err
		}

		// El respaldo legado se arrastra TAL CUAL (el dominio nunca lo escribe).
		var legacy []byte
		if hadPrev {
			legacy = prev.SecretsEncLegacy
		}

		// Reemplaza origen y acciones.
		if err := tx.Where("module_id = ?", dr.ID).Delete(&dataSourceRow{}).Error; err != nil {
			return fmt.Errorf("modules: error limpiando origen previo: %w", err)
		}
		if err := tx.Where("module_id = ?", dr.ID).Delete(&actionRow{}).Error; err != nil {
			return fmt.Errorf("modules: error limpiando acciones previas: %w", err)
		}
		if err := insertDataSource(tx, dr.ID, def, sealed, ptrOrNil(def.DataSource.ConnectionTemplateID), legacy); err != nil {
			return err
		}
		if err := insertActions(tx, dr.ID, def.Actions); err != nil {
			return err
		}
		return nil
	})
}

// Delete elimina la definición por slug (cascade elimina origen y acciones).
func (r *gormRepository) Delete(ctx context.Context, slug string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dr definitionRow
		if err := tx.Where("slug = ?", slug).First(&dr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("modules: error buscando módulo a eliminar: %w", err)
		}
		if err := tx.Where("id = ?", dr.ID).Delete(&definitionRow{}).Error; err != nil {
			return fmt.Errorf("modules: error eliminando módulo: %w", err)
		}
		return nil
	})
}

// sealSecrets serializa y cifra el bundle de credenciales. Devuelve nil si el
// bundle está vacío (origen sin credenciales, p. ej. auth none).
func (r *gormRepository) sealSecrets(sec Secrets) ([]byte, error) {
	if sec.isEmpty() {
		return nil, nil
	}
	plain, err := json.Marshal(sec)
	if err != nil {
		return nil, fmt.Errorf("modules: no se pudieron serializar las credenciales: %w", err)
	}
	sealed, err := r.sealer.Seal(plain)
	if err != nil {
		return nil, fmt.Errorf("modules: no se pudieron cifrar las credenciales: %w", err)
	}
	return sealed, nil
}

// insertDataSource crea la fila del origen del módulo con los valores VARIABLE
// ya cifrados (sealed puede ser nil), preservando la FK a la plantilla y el
// respaldo legado (connectionTemplateID / secretsEncLegacy pueden ser nil).
func insertDataSource(tx *gorm.DB, moduleID string, def *Definition, sealed []byte, connectionTemplateID *string, secretsEncLegacy []byte) error {
	ds := dataSourceRow{
		ModuleID:             moduleID,
		SourceType:           def.SourceType,
		SecretsEnc:           sealed,
		PGWhitelist:          encodeWhitelist(def.DataSource.PGWhitelist),
		ConnectionTemplateID: connectionTemplateID,
		SecretsEncLegacy:     secretsEncLegacy,
	}
	if err := tx.Create(&ds).Error; err != nil {
		return fmt.Errorf("modules: error creando origen del módulo: %w", err)
	}
	return nil
}

// ptrOrNil devuelve un puntero al string, o nil si está vacío (para escribir
// NULL en la FK cuando corresponda).
func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}

// validateTemplateRef valida la invariante de escritura: la FK a la plantilla es
// obligatoria y debe existir (Req. 7.1). Rechaza con ErrTemplateRequired si
// falta o no existe.
func (r *gormRepository) validateTemplateRef(ctx context.Context, templateID string) error {
	if strings.TrimSpace(templateID) == "" {
		return ErrTemplateRequired
	}
	exists, err := r.connRepo.TemplateExists(ctx, templateID)
	if err != nil {
		return fmt.Errorf("modules: error validando la plantilla de conexión: %w", err)
	}
	if !exists {
		return ErrTemplateRequired
	}
	return nil
}

// mergeVariableValues aplica, por cada clave VARIABLE declarada (keys), la regla
// de merge contra el bundle previo descifrado: vacío/ausente = conservar;
// presente = reemplazar; clave fuera de keys = descartar. Re-sella el bundle
// COMPLETO. Es una función pura (recibe el Sealer por inyección). Devuelve nil si
// el merge resulta vacío (módulo sin variables, p. ej. mapeo 1:1 de migración).
func mergeVariableValues(sealer *secrets.Sealer, prevEnc []byte, sec Secrets, keys []string) ([]byte, error) {
	prev := map[string]string{}
	if len(prevEnc) > 0 {
		plain, err := sealer.Open(prevEnc)
		if err != nil {
			return nil, fmt.Errorf("modules: no se pudieron descifrar los valores variables previos")
		}
		var bundle Secrets
		if err := json.Unmarshal(plain, &bundle); err != nil {
			return nil, fmt.Errorf("modules: valores variables previos con formato inválido")
		}
		if bundle.VariableValues != nil {
			prev = bundle.VariableValues
		}
	}

	merged := map[string]string{}
	for _, k := range keys {
		newVal := strings.TrimSpace(sec.VariableValues[k])
		if newVal != "" {
			merged[k] = newVal
			continue
		}
		if prevVal, ok := prev[k]; ok {
			merged[k] = prevVal
		}
	}

	if len(merged) == 0 {
		return nil, nil
	}
	plain, err := json.Marshal(Secrets{VariableValues: merged})
	if err != nil {
		return nil, fmt.Errorf("modules: no se pudieron serializar los valores variables: %w", err)
	}
	sealed, err := sealer.Seal(plain)
	if err != nil {
		return nil, fmt.Errorf("modules: no se pudieron cifrar los valores variables: %w", err)
	}
	return sealed, nil
}

// insertActions persiste las acciones del módulo.
func insertActions(tx *gorm.DB, moduleID string, actions []Action) error {
	if len(actions) == 0 {
		return nil
	}
	rows := make([]actionRow, 0, len(actions))
	for _, a := range actions {
		rows = append(rows, actionRow{
			ModuleID:    moduleID,
			ActionKey:   a.ActionKey,
			Kind:        a.Kind,
			Label:       a.Label,
			Placement:   a.Placement,
			Destructive: a.Destructive,
			PermAction:  a.PermAction,
			IsWrite:     a.IsWrite,
			SQLOp:       a.SQLOp,
			InputFields: encodeJSONArray(a.InputFields),
			Config:      encodeActionConfig(a),
		})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("modules: error creando acciones del módulo: %w", err)
	}
	return nil
}

// whitelistSetOf devuelve el conjunto (set) de columnas permitidas de `table`
// en la lista blanca `wl`, y un bool que indica si la tabla tiene entrada. Es la
// extracción a función de paquete del helper que antes vivía como closure dentro
// de validateMultiTable; su comportamiento es idéntico (tabla sin entrada ⇒
// (nil, false)).
func whitelistSetOf(wl map[string][]string, table string) (map[string]struct{}, bool) {
	cols, ok := wl[table]
	if !ok {
		return nil, false
	}
	set := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		set[c] = struct{}{}
	}
	return set, true
}

// mergeAuxTablesIntoWhitelist es el ÚNICO punto que funde las tablas auxiliares
// (catálogos declarados en fetchMapping.auxTables) en la lista blanca efectiva
// del módulo ANTES de validar y de persistir. Por cada auxiliar añade sus
// columnas a def.DataSource.PGWhitelist[table] como UNIÓN sin duplicados, de modo
// que validateMultiTable, validateColumnValueSources, el motor de listado y el
// endpoint de options vean UNA sola whitelist. Antes valida las guardas de las
// auxiliares (table/columns no vacías; sin choque de nombre con la principal o
// una foránea). Las auxiliares NO entran a spec.Tables(), por lo que no disparan
// el grafo conexo de BuildRelationTree. Devuelve ErrMultiTableValidation (→ E422)
// ante una auxiliar inválida.
func mergeAuxTablesIntoWhitelist(def *Definition) error {
	spec := MultiTableFromFetchMapping(def.FetchMapping)
	if len(spec.AuxTables) == 0 {
		return nil
	}

	// Conjunto de tablas del árbol (principal + foráneas): una auxiliar no puede
	// chocar con ellas (son conceptos distintos: catálogo vs tabla del grid).
	treeTables := make(map[string]struct{}, len(spec.Tables()))
	for _, t := range spec.Tables() {
		treeTables[t] = struct{}{}
	}

	for _, aux := range spec.AuxTables {
		if aux.Table == "" {
			return fmt.Errorf("%w: una tabla auxiliar no declara nombre de tabla", ErrMultiTableValidation)
		}
		if len(aux.Columns) == 0 {
			return fmt.Errorf("%w: la tabla auxiliar %q no declara columnas", ErrMultiTableValidation, aux.Table)
		}
		if _, clash := treeTables[aux.Table]; clash {
			return fmt.Errorf("%w: la tabla auxiliar %q choca con la tabla principal o una foránea del módulo", ErrMultiTableValidation, aux.Table)
		}
	}

	// Fusión: unión sin duplicados de las columnas auxiliares en la whitelist.
	if def.DataSource.PGWhitelist == nil {
		def.DataSource.PGWhitelist = map[string][]string{}
	}
	for table, cols := range spec.AuxWhitelist() {
		existing := def.DataSource.PGWhitelist[table]
		seen := make(map[string]struct{}, len(existing))
		for _, c := range existing {
			seen[c] = struct{}{}
		}
		for _, c := range cols {
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			existing = append(existing, c)
		}
		def.DataSource.PGWhitelist[table] = existing
	}
	return nil
}

// validateDefinition valida las invariantes de una definición antes de persistir.
func validateDefinition(def *Definition) error {
	if !auth.IsValidSlug(def.Slug) {
		return ErrInvalidSlug
	}
	if strings.TrimSpace(def.Name) == "" {
		return ErrEmptyName
	}
	// Solo se admite el origen PostgreSQL: el origen "API HTTP" fue retirado del
	// constructor de módulos, por lo que ya no pueden crearse ni actualizarse
	// definiciones de ese tipo.
	if def.SourceType != SourcePostgres {
		return fmt.Errorf("modules: tipo de origen no soportado %q; solo se permite %q", def.SourceType, SourcePostgres)
	}
	// Funde las tablas auxiliares (catálogos) en la lista blanca efectiva ANTES de
	// cualquier validación que la consulte. Es el único punto de fusión: tras él,
	// validateMultiTable/validateColumnValueSources, el motor y el endpoint de
	// options ven una sola whitelist (con los catálogos ya incluidos), y la
	// persistencia (insertDataSource) escribe esa misma whitelist. Las auxiliares
	// no entran a Tables(), así que no disparan el grafo conexo.
	if err := mergeAuxTablesIntoWhitelist(def); err != nil {
		return err
	}
	// Invariante del rediseño columna-céntrico: un módulo con acciones de editar
	// o eliminar DEBE declarar al menos una columna llave primaria (el WHERE de
	// UPDATE/DELETE se deriva de ella). No se acepta el fallback heredado isKey al
	// guardar: ese fallback es solo para EJECUCIÓN de definiciones no re-guardadas.
	if hasWriteActionEditOrDelete(def.Actions) &&
		len(PrimaryKeyColumns(ColumnsFromDisplayMapping(def.DisplayMapping))) == 0 {
		return ErrNoPrimaryKey
	}
	// Validación multi-tabla (reglas a–j). Para módulos N=1 (sin foráneas ni
	// relaciones) es un no-op salvo las comprobaciones que degradan igual que
	// hoy. Todos los rechazos envuelven ErrMultiTableValidation (→ E422).
	if err := validateMultiTable(def); err != nil {
		return err
	}
	// Validación INCONDICIONAL de la fuente de valor por columna (cierra el hueco
	// N=1: validateMultiTable es no-op para módulos de una sola tabla, así que la
	// whitelist de los catálogos DINÁMICOS de columna debe comprobarse también
	// aquí). Cada fuente dynamic debe apuntar a una tabla con entrada en la lista
	// blanca y a columnas id/label no vacías y whitelisteadas. Todo rechazo
	// envuelve ErrMultiTableValidation (→ E422).
	if err := validateColumnValueSources(def); err != nil {
		return err
	}
	// Validación de las estrategias de llenado (fail-fast al GUARDAR, §12.A):
	// camino transaccional, invariante de emisión (columna destino ⇔ FieldSpec),
	// prefijo reservado __capture_, unicidad/resolución de CaptureKey y whitelist
	// de catálogos/saltos. Todo rechazo envuelve ErrMultiTableValidation (→ E422).
	if err := validateFillStrategies(def); err != nil {
		return err
	}
	return nil
}

// validateFillStrategies valida, para cada acción, los campos con
// Source=="fillStrategy" (y la obligación de camino transaccional de una acción
// con estrategias). Es la PRIMERA red (el motor es la segunda, §8.1); cada
// rechazo envuelve ErrMultiTableValidation para que writeDefError lo mapee a
// E422_VALIDATION. Reglas (design.md §12.A):
//
//  1. Camino transaccional: una acción con ≥1 campo fillStrategy exige
//     TargetTable != "" (el INSERT transaccional corre dentro de la tx del
//     camino multi-tabla). La condición es TargetTable != "" && HasFillStrategies,
//     NO IsMultiTable() (desacople, MEDIUM #1).
//  2. Invariante de emisión: derive-via-fk ⇒ len(Mapping)==1 y
//     Mapping[0].TargetColumn == Name; derive-from-option / capture que escribe ⇒
//     Name ∈ whitelist[TargetTable], salvo prefijo __capture_.
//  3. Ningún Name con prefijo __capture_ coincide con columna de TargetTable.
//  4. CaptureKey únicos por acción; todo SourceCaptureKey/Jump.LocalCaptureKey
//     resuelve a un CaptureKey declarado en la misma acción.
//  5. Whitelist de Catalog.Table/IDColumn/LabelColumn/StoreColumn, Attribute de
//     derive-from-option, Jump.RelatedTable/RelatedColumn/Mapping[].Attribute; y
//     Jump.LocalAttribute en whitelist[Catalog.Table de la captura referida por
//     Jump.LocalCaptureKey] (MEDIUM #3, INV-7).
func validateFillStrategies(def *Definition) error {
	whitelist := def.DataSource.PGWhitelist
	for _, act := range def.Actions {
		// Se valida cualquier acción con estrategias de llenado O con unicidad: ambas
		// activan el camino transaccional del motor (INV-3) y por tanto exigen
		// TargetTable (design.md §8).
		if !HasFillStrategies(act) && act.Uniqueness == nil {
			continue
		}

		// Regla 1: camino transaccional. Una acción con estrategias o con unicidad
		// debe declarar la tabla objetivo del INSERT.
		if strings.TrimSpace(act.TargetTable) == "" {
			return fmt.Errorf("%w: la acción %q usa estrategias de llenado o unicidad y debe declarar una tabla objetivo (camino transaccional)", ErrMultiTableValidation, act.ActionKey)
		}
		target := act.TargetTable
		targetSet, ok := whitelistSetOf(whitelist, target)
		if !ok {
			return fmt.Errorf("%w: la tabla objetivo %q de la acción %q no tiene entrada en la lista blanca", ErrMultiTableValidation, target, act.ActionKey)
		}

		// Recopila las capturas (CaptureKey ⇒ catálogo) para resolver referencias
		// y la whitelist del LocalAttribute de los saltos (regla 4 y 5).
		captureCatalog := make(map[string]*DynamicSource)
		for _, f := range act.InputFields {
			if f.Source != FieldSourceFillStrategy || f.Strategy == nil {
				continue
			}
			s := f.Strategy
			if s.Kind != StrategyCaptureFromCatalog {
				continue
			}
			// Regla 4: CaptureKey único.
			if s.CaptureKey != "" {
				if _, dup := captureCatalog[s.CaptureKey]; dup {
					return fmt.Errorf("%w: la acción %q declara el captureKey %q más de una vez", ErrMultiTableValidation, act.ActionKey, s.CaptureKey)
				}
			}
			captureCatalog[s.CaptureKey] = s.Catalog
		}

		for _, f := range act.InputFields {
			if f.Source != FieldSourceFillStrategy {
				continue
			}
			if f.Strategy == nil {
				return fmt.Errorf("%w: el campo %q de la acción %q usa fillStrategy pero no declara estrategia", ErrMultiTableValidation, f.Name, act.ActionKey)
			}
			s := f.Strategy
			isCapture := f.IsCaptureOnly()

			// Regla 3: ningún __capture_ coincide con columna de la tabla objetivo.
			if isCapture {
				if _, isCol := targetSet[f.Name]; isCol {
					return fmt.Errorf("%w: el campo capturado %q de la acción %q colisiona con una columna de la tabla objetivo %q", ErrMultiTableValidation, f.Name, act.ActionKey, target)
				}
			}

			switch s.Kind {
			case StrategyCaptureFromCatalog:
				// Regla 2: una captura que ESCRIBE (name no __capture_) debe ser
				// columna de la tabla objetivo.
				if !isCapture {
					if _, isCol := targetSet[f.Name]; !isCol {
						return fmt.Errorf("%w: el campo %q (capture-from-catalog) de la acción %q no está en la lista blanca de la tabla objetivo %q", ErrMultiTableValidation, f.Name, act.ActionKey, target)
					}
				}
				// Regla 5: whitelist del catálogo (tabla/id/label/storeColumn).
				if err := validateStrategyCatalog(whitelist, act.ActionKey, f.Name, s); err != nil {
					return err
				}
			case StrategyDeriveFromOption:
				// Regla 2: escribe a columna; name debe ser columna de la tabla objetivo.
				if _, isCol := targetSet[f.Name]; !isCol {
					return fmt.Errorf("%w: el campo %q (derive-from-option) de la acción %q no está en la lista blanca de la tabla objetivo %q", ErrMultiTableValidation, f.Name, act.ActionKey, target)
				}
				// Regla 4: SourceCaptureKey resuelve a una captura declarada.
				cat, ok := captureCatalog[s.SourceCaptureKey]
				if !ok {
					return fmt.Errorf("%w: el campo %q (derive-from-option) de la acción %q referencia el captureKey %q sin declarar", ErrMultiTableValidation, f.Name, act.ActionKey, s.SourceCaptureKey)
				}
				// Regla 5: Attribute en la whitelist del catálogo capturado.
				if err := validateAttributeInCatalog(whitelist, act.ActionKey, f.Name, cat, s.Attribute); err != nil {
					return err
				}
			case StrategyDeriveViaFK:
				// Regla 2: len(Mapping)==1 y Mapping[0].TargetColumn == Name.
				if len(s.Mapping) != 1 {
					return fmt.Errorf("%w: el campo %q (derive-via-fk) de la acción %q debe declarar exactamente un mapeo atributo→columna", ErrMultiTableValidation, f.Name, act.ActionKey)
				}
				if s.Mapping[0].TargetColumn != f.Name {
					return fmt.Errorf("%w: el campo %q (derive-via-fk) de la acción %q debe mapear a su propia columna (targetColumn == name)", ErrMultiTableValidation, f.Name, act.ActionKey)
				}
				if _, isCol := targetSet[f.Name]; !isCol {
					return fmt.Errorf("%w: el campo %q (derive-via-fk) de la acción %q no está en la lista blanca de la tabla objetivo %q", ErrMultiTableValidation, f.Name, act.ActionKey, target)
				}
				if s.Jump == nil {
					return fmt.Errorf("%w: el campo %q (derive-via-fk) de la acción %q no declara el salto", ErrMultiTableValidation, f.Name, act.ActionKey)
				}
				// Regla 4: Jump.LocalCaptureKey resuelve a una captura declarada.
				localCat, ok := captureCatalog[s.Jump.LocalCaptureKey]
				if !ok {
					return fmt.Errorf("%w: el campo %q (derive-via-fk) de la acción %q referencia el captureKey local %q sin declarar", ErrMultiTableValidation, f.Name, act.ActionKey, s.Jump.LocalCaptureKey)
				}
				// Regla 5: whitelist de RelatedTable/RelatedColumn/Mapping.Attribute.
				relSet, ok := whitelistSetOf(whitelist, s.Jump.RelatedTable)
				if !ok {
					return fmt.Errorf("%w: la tabla relacionada %q del campo %q (acción %q) no tiene entrada en la lista blanca", ErrMultiTableValidation, s.Jump.RelatedTable, f.Name, act.ActionKey)
				}
				for _, col := range []string{s.Jump.RelatedColumn, s.Mapping[0].Attribute} {
					if _, ok := relSet[col]; !ok {
						return fmt.Errorf("%w: la columna %q.%q del salto del campo %q (acción %q) no está en la lista blanca", ErrMultiTableValidation, s.Jump.RelatedTable, col, f.Name, act.ActionKey)
					}
				}
				// Regla 5 (INV-7): Jump.LocalAttribute en la whitelist del catálogo
				// de la captura local referida.
				if err := validateAttributeInCatalog(whitelist, act.ActionKey, f.Name, localCat, s.Jump.LocalAttribute); err != nil {
					return err
				}
			case StrategyConstant, StrategyAutogenerated:
				// Sin referencias ni columnas de catálogo que validar aquí.
			default:
				return fmt.Errorf("%w: el campo %q de la acción %q declara una estrategia de llenado desconocida %q", ErrMultiTableValidation, f.Name, act.ActionKey, s.Kind)
			}
		}

		// Unicidad (FEAT-002, design.md §8): cada columna de la combinación debe
		// estar en la lista blanca de la tabla objetivo. La obligación de camino
		// transaccional (Uniqueness != nil exige TargetTable) ya la cubre la Regla 1
		// arriba (se entra a este bloque solo con TargetTable no vacía).
		if act.Uniqueness != nil {
			if len(act.Uniqueness.Columns) == 0 {
				return fmt.Errorf("%w: la acción %q declara unicidad sin columnas", ErrMultiTableValidation, act.ActionKey)
			}
			for _, col := range act.Uniqueness.Columns {
				if _, isCol := targetSet[col]; !isCol {
					return fmt.Errorf("%w: la columna de unicidad %q de la acción %q no está en la lista blanca de la tabla objetivo %q", ErrMultiTableValidation, col, act.ActionKey, target)
				}
			}
		}
	}
	return nil
}

// validateStrategyCatalog valida que el catálogo de una captura (tabla, id,
// etiqueta y storeColumn si se declara) esté en la lista blanca del módulo.
func validateStrategyCatalog(whitelist map[string][]string, actionKey, fieldName string, s *FillStrategy) error {
	if s.Catalog == nil {
		return fmt.Errorf("%w: el campo %q (capture-from-catalog) de la acción %q no declara catálogo", ErrMultiTableValidation, fieldName, actionKey)
	}
	cat := s.Catalog
	set, ok := whitelistSetOf(whitelist, cat.Table)
	if !ok {
		return fmt.Errorf("%w: la tabla de catálogo %q del campo %q (acción %q) no tiene entrada en la lista blanca", ErrMultiTableValidation, cat.Table, fieldName, actionKey)
	}
	if cat.IDColumn == "" || cat.LabelColumn == "" {
		return fmt.Errorf("%w: el catálogo del campo %q (acción %q) debe declarar columna de id y de etiqueta", ErrMultiTableValidation, fieldName, actionKey)
	}
	cols := []string{cat.IDColumn, cat.LabelColumn}
	if s.StoreColumn != "" {
		cols = append(cols, s.StoreColumn)
	}
	for _, col := range cols {
		if _, ok := set[col]; !ok {
			return fmt.Errorf("%w: la columna %q.%q del catálogo del campo %q (acción %q) no está en la lista blanca", ErrMultiTableValidation, cat.Table, col, fieldName, actionKey)
		}
	}
	return nil
}

// validateAttributeInCatalog valida que un atributo (de derive-from-option o el
// LocalAttribute de un salto) esté en la whitelist de la tabla del catálogo
// referido por la captura.
func validateAttributeInCatalog(whitelist map[string][]string, actionKey, fieldName string, cat *DynamicSource, attribute string) error {
	if cat == nil {
		return fmt.Errorf("%w: el campo %q de la acción %q referencia una captura sin catálogo", ErrMultiTableValidation, fieldName, actionKey)
	}
	set, ok := whitelistSetOf(whitelist, cat.Table)
	if !ok {
		return fmt.Errorf("%w: la tabla de catálogo %q referida por el campo %q (acción %q) no tiene entrada en la lista blanca", ErrMultiTableValidation, cat.Table, fieldName, actionKey)
	}
	if _, ok := set[attribute]; !ok {
		return fmt.Errorf("%w: el atributo %q.%q referido por el campo %q (acción %q) no está en la lista blanca", ErrMultiTableValidation, cat.Table, attribute, fieldName, actionKey)
	}
	return nil
}

// validateColumnValueSources recorre TODAS las columnas resueltas (principal
// incluida) y, por cada una con una fuente de valor DINÁMICA, valida contra la
// lista blanca del módulo que la tabla del catálogo existe y que sus columnas id
// y etiqueta no son vacías y están whitelisteadas. Es independiente de que el
// módulo sea multi-tabla o N=1. Los fallos envuelven ErrMultiTableValidation.
func validateColumnValueSources(def *Definition) error {
	whitelist := def.DataSource.PGWhitelist
	principal := MultiTableFromFetchMapping(def.FetchMapping).PrincipalTable
	cols := ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, principal)
	for _, c := range cols {
		vs := c.ValueSource
		if vs == nil || vs.Kind != "dynamic" || vs.DynamicSource == nil {
			continue
		}
		ds := vs.DynamicSource
		set, ok := whitelistSetOf(whitelist, ds.Table)
		if !ok {
			return fmt.Errorf("%w: la tabla de catálogo %q de la columna %q no tiene entrada en la lista blanca", ErrMultiTableValidation, ds.Table, c.Field)
		}
		if ds.IDColumn == "" || ds.LabelColumn == "" {
			return fmt.Errorf("%w: la fuente dinámica de la columna %q debe declarar columna de id y de etiqueta", ErrMultiTableValidation, c.Field)
		}
		for _, col := range []string{ds.IDColumn, ds.LabelColumn} {
			if _, ok := set[col]; !ok {
				return fmt.Errorf("%w: la columna %q.%q del catálogo de la columna %q no está en la lista blanca", ErrMultiTableValidation, ds.Table, col, c.Field)
			}
		}
	}
	return nil
}

// validateMultiTable aplica las reglas de guardado específicas de módulos
// multi-tabla (reglas a–j del diseño §4). Si el módulo NO es multi-tabla, solo
// verifica que no haya columnas con sourceTable apuntando fuera de la principal
// (defensa) y retorna nil. CADA rechazo envuelve ErrMultiTableValidation con %w,
// conservando el mensaje español específico para que writeDefError lo mapee a
// E422_VALIDATION.
func validateMultiTable(def *Definition) error {
	spec := MultiTableFromFetchMapping(def.FetchMapping)
	if !spec.IsMultiTable() {
		return nil
	}

	whitelist := def.DataSource.PGWhitelist
	cols := ColumnsFromDisplayMappingWithPrincipal(def.DisplayMapping, spec.PrincipalTable)

	// (i)/(b)/(c) Árbol: DAG por FK + conexo; tabla huérfana; relación a tabla no
	// declarada. BuildRelationTree ya envuelve ErrMultiTableValidation.
	tree, err := BuildRelationTree(spec)
	if err != nil {
		return err
	}

	declaredTables := make(map[string]struct{}, len(spec.Tables()))
	for _, t := range spec.Tables() {
		declaredTables[t] = struct{}{}
	}

	// (d) Lista blanca: cada tabla referenciada debe tener entrada en PGWhitelist.
	whitelistSet := func(table string) (map[string]struct{}, bool) {
		return whitelistSetOf(whitelist, table)
	}
	for _, t := range spec.Tables() {
		if _, ok := whitelist[t]; !ok {
			return fmt.Errorf("%w: la tabla %q no tiene entrada en la lista blanca", ErrMultiTableValidation, t)
		}
	}

	// (g) joinType válido; (d) columnas del ON whitelisteadas por tabla.
	for _, r := range spec.Relationships {
		if r.JoinType != JoinInner && r.JoinType != JoinLeft {
			return fmt.Errorf("%w: tipo de JOIN %q no permitido (use inner o left)", ErrMultiTableValidation, r.JoinType)
		}
		childSet, okC := whitelistSet(r.ChildTable)
		parentSet, okP := whitelistSet(r.ParentTable)
		if !okC {
			return fmt.Errorf("%w: la tabla hija %q no tiene entrada en la lista blanca", ErrMultiTableValidation, r.ChildTable)
		}
		if !okP {
			return fmt.Errorf("%w: la tabla padre %q no tiene entrada en la lista blanca", ErrMultiTableValidation, r.ParentTable)
		}
		if len(r.On) == 0 {
			return fmt.Errorf("%w: la relación %q→%q no declara columnas de unión (ON)", ErrMultiTableValidation, r.ChildTable, r.ParentTable)
		}
		for _, p := range r.On {
			if _, ok := childSet[p.Child]; !ok {
				return fmt.Errorf("%w: la columna de unión %q.%q no está en la lista blanca", ErrMultiTableValidation, r.ChildTable, p.Child)
			}
			if _, ok := parentSet[p.Parent]; !ok {
				return fmt.Errorf("%w: la columna de unión %q.%q no está en la lista blanca", ErrMultiTableValidation, r.ParentTable, p.Parent)
			}
		}
		// Constantes del ON: la tabla debe ser una de las dos de la relación y la
		// columna debe estar en la lista blanca de esa tabla. El valor es un literal
		// que Postgres coacciona; no se valida su tipo aquí.
		for _, jc := range r.OnConstants {
			if jc.Table != r.ChildTable && jc.Table != r.ParentTable {
				return fmt.Errorf("%w: la constante de unión %q.%q no pertenece a la relación %q→%q",
					ErrMultiTableValidation, jc.Table, jc.Column, r.ChildTable, r.ParentTable)
			}
			set := childSet
			if jc.Table == r.ParentTable {
				set = parentSet
			}
			if _, ok := set[jc.Column]; !ok {
				return fmt.Errorf("%w: la columna de unión %q.%q no está en la lista blanca",
					ErrMultiTableValidation, jc.Table, jc.Column)
			}
		}
	}

	// Índice de columnas por (sourceTable, field) y conteo de homónimos por field.
	colByKey := make(map[[2]string]ResolvedColumn, len(cols))
	countByField := make(map[string]int, len(cols))
	for _, c := range cols {
		colByKey[[2]string{c.SourceTable, c.Field}] = c
		countByField[c.Field]++
	}

	// (d) columnas de visualización/PK whitelisteadas por su tabla; (j) alias ≤63.
	for _, c := range cols {
		set, ok := whitelistSet(c.SourceTable)
		if !ok {
			return fmt.Errorf("%w: la columna %q referencia la tabla %q sin entrada en la lista blanca", ErrMultiTableValidation, c.Field, c.SourceTable)
		}
		if _, ok := set[c.Field]; !ok {
			return fmt.Errorf("%w: la columna %q.%q no está en la lista blanca", ErrMultiTableValidation, c.SourceTable, c.Field)
		}
		// (j) longitud de alias en BYTES: len(tabla)+2+len(col) ≤ 63.
		if aliasLen := len(c.SourceTable) + 2 + len(c.Field); aliasLen > 63 {
			return fmt.Errorf("%w: el alias de salida %q.%q excede 63 bytes; acorta el nombre de tabla o columna", ErrMultiTableValidation, c.SourceTable, c.Field)
		}
	}

	// (h)/(d) condiciones de listado: cualificadas y whitelisteadas.
	for _, cond := range def.ListConditions {
		if cond.Field == "" {
			continue
		}
		table := cond.SourceTable
		if table == "" {
			if countByField[cond.Field] > 1 {
				return fmt.Errorf("%w: la condición %q es ambigua: existe en varias tablas, indica la tabla de origen", ErrMultiTableValidation, cond.Field)
			}
			// Única o inexistente: resolver por su columna si existe.
			for _, c := range cols {
				if c.Field == cond.Field {
					table = c.SourceTable
					break
				}
			}
			if table == "" {
				table = spec.PrincipalTable
			}
		}
		set, ok := whitelistSet(table)
		if !ok {
			return fmt.Errorf("%w: la condición %q referencia la tabla %q sin entrada en la lista blanca", ErrMultiTableValidation, cond.Field, table)
		}
		if _, ok := set[cond.Field]; !ok {
			return fmt.Errorf("%w: la columna de condición %q.%q no está en la lista blanca", ErrMultiTableValidation, table, cond.Field)
		}
	}

	// Reglas por acción: (a) PK por tabla escribible edit/delete; (a2) objetivos
	// de edit coherentes; (a3) objetivos de create; (e)/(f) target/tope válidos;
	// (h) ambigüedad en InputFields; (d) catálogos dinámicos whitelisteados.
	pkByTable := PrimaryKeyColumnsByTable(cols)
	for _, act := range def.Actions {
		// Catálogos de orígenes dinámicos (todas las acciones).
		for _, f := range act.InputFields {
			if f.Source != FieldSourceDynamic || f.DynamicSource == nil {
				continue
			}
			ds := f.DynamicSource
			set, ok := whitelistSet(ds.Table)
			if !ok {
				return fmt.Errorf("%w: la tabla de catálogo %q del campo %q no tiene entrada en la lista blanca", ErrMultiTableValidation, ds.Table, f.Name)
			}
			for _, col := range []string{ds.IDColumn, ds.LabelColumn} {
				if col == "" {
					continue
				}
				if _, ok := set[col]; !ok {
					return fmt.Errorf("%w: la columna %q.%q del catálogo no está en la lista blanca", ErrMultiTableValidation, ds.Table, col)
				}
			}
		}

		switch act.Kind {
		case ActionKindEdit:
			// (a2) cada InputField objetivo existe, es editable y no-PK; (h) homónima.
			affected := make(map[string]struct{})
			for _, f := range act.InputFields {
				// Solo columnas objetivo de escritura (input/constant/select/dynamic).
				if f.Source == FieldSourceRow {
					continue
				}
				table := f.SourceTable
				if table == "" {
					if countByField[f.Name] > 1 {
						return fmt.Errorf("%w: el campo %q es ambiguo: existe en varias tablas, indica la tabla de origen", ErrMultiTableValidation, f.Name)
					}
					for _, c := range cols {
						if c.Field == f.Name {
							table = c.SourceTable
							break
						}
					}
				}
				col, ok := colByKey[[2]string{table, f.Name}]
				if !ok || table == "" {
					return fmt.Errorf("%w: el campo %q no corresponde a una columna de la tabla %q", ErrMultiTableValidation, f.Name, table)
				}
				if col.IsPrimaryKey || !col.Editable {
					return fmt.Errorf("%w: el campo %q no corresponde a una columna editable de la tabla %q", ErrMultiTableValidation, f.Name, table)
				}
				affected[table] = struct{}{}
			}
			// (a) cada tabla afectada por el edit debe tener PK.
			for table := range affected {
				if len(pkByTable[table]) == 0 {
					return fmt.Errorf("%w: la tabla %q no declara llave primaria para la acción de editar", ErrMultiTableValidation, table)
				}
			}
		case ActionKindDelete:
			// (e) tope válido; conjunto derivable; (a) PK por tabla del conjunto.
			top := act.DeleteTopTable
			if top == "" {
				top = spec.PrincipalTable
			}
			if _, ok := declaredTables[top]; !ok {
				return fmt.Errorf("%w: la tabla tope %q de la acción de eliminar no pertenece al módulo", ErrMultiTableValidation, top)
			}
			order, derr := tree.DeleteOrder(top)
			if derr != nil {
				return derr
			}
			for _, table := range order {
				if len(pkByTable[table]) == 0 {
					return fmt.Errorf("%w: la tabla %q del conjunto de borrado no declara llave primaria", ErrMultiTableValidation, table)
				}
			}
		case ActionKindCreate:
			// (f) target válido; (a3) objetivos en whitelist[target] y con sourceTable==target.
			target := act.TargetTable
			if target == "" {
				target = spec.PrincipalTable
			}
			if _, ok := declaredTables[target]; !ok {
				return fmt.Errorf("%w: la tabla objetivo %q de la acción de crear no pertenece al módulo", ErrMultiTableValidation, target)
			}
			set, ok := whitelistSet(target)
			if !ok {
				return fmt.Errorf("%w: la tabla objetivo %q no tiene entrada en la lista blanca", ErrMultiTableValidation, target)
			}
			for _, f := range act.InputFields {
				if f.Source == FieldSourceRow {
					continue
				}
				// Las capturas que NO escriben a columna (campos solo-captura:
				// prefijo reservado __capture_ o flag explícito CaptureOnly) NO son
				// columnas de la tabla objetivo: su valor solo alimenta derivaciones
				// y la proyección final del motor las descarta antes del INSERT. Se
				// eximen de la comprobación "name ∈ whitelist[target]"/"name es
				// columna"; su whitelist de catálogo la valida validateFillStrategies.
				if f.IsCaptureOnly() {
					continue
				}
				if _, ok := set[f.Name]; !ok {
					return fmt.Errorf("%w: el campo %q no está en la lista blanca de la tabla objetivo %q", ErrMultiTableValidation, f.Name, target)
				}
				if _, ok := colByKey[[2]string{target, f.Name}]; !ok {
					return fmt.Errorf("%w: el campo %q no corresponde a una columna de la tabla objetivo %q", ErrMultiTableValidation, f.Name, target)
				}
			}
		}
	}

	return nil
}

// defaultCountryScope normaliza el ámbito de país; vacío cae a "none".
func defaultCountryScope(scope string) string {
	if scope == CountryScopeMulti {
		return CountryScopeMulti
	}
	return CountryScopeNone
}

// isEmpty indica que no se aportan valores VARIABLE ("sin cambios" en TODAS las
// claves): bundle ausente o vacío. Nunca significa "borrar todas las variables".
func (s Secrets) isEmpty() bool {
	return len(s.VariableValues) == 0
}

// toActionDomain convierte una fila de acción al modelo de dominio.
func toActionDomain(ar actionRow) Action {
	targetTable, uniqueness, deleteTopTable := decodeActionConfig(ar.Config)
	return Action{
		ID:             ar.ID,
		ActionKey:      ar.ActionKey,
		Kind:           ar.Kind,
		Label:          ar.Label,
		Placement:      ar.Placement,
		Destructive:    ar.Destructive,
		PermAction:     ar.PermAction,
		IsWrite:        ar.IsWrite,
		SQLOp:          ar.SQLOp,
		InputFields:    decodeFieldSpecs(ar.InputFields),
		TargetTable:    targetTable,
		Uniqueness:     uniqueness,
		DeleteTopTable: deleteTopTable,
	}
}

// --- helpers de (de)serialización JSONB ------------------------------------

func encodeJSONObject(m map[string]any) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func decodeJSONObject(b []byte) map[string]any {
	if len(b) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func encodeJSONArray(v []FieldSpec) []byte {
	if len(v) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func decodeFieldSpecs(b []byte) []FieldSpec {
	if len(b) == 0 {
		return nil
	}
	var v []FieldSpec
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return v
}

// actionConfig es la forma serializada de la configuración extendida de una
// acción en la columna JSONB `config`. Todas las claves son omitempty para que
// una acción sin configuración extendida produzca exactamente "{}" (retrocompat
// byte-por-byte con filas antiguas).
type actionConfig struct {
	TargetTable    string           `json:"targetTable,omitempty"`
	Uniqueness     *UniquenessCheck `json:"uniqueness,omitempty"`
	DeleteTopTable string           `json:"deleteTopTable,omitempty"`
}

// encodeActionConfig serializa la configuración extendida de una acción
// (targetTable, uniqueness, deleteTopTable) a JSONB. Emite SOLO las claves
// presentes; una acción sin ninguna de ellas produce "{}" (byte-por-byte igual
// que las filas antiguas). Es el inverso de decodeActionConfig.
func encodeActionConfig(a Action) []byte {
	cfg := actionConfig{
		TargetTable:    a.TargetTable,
		Uniqueness:     a.Uniqueness,
		DeleteTopTable: a.DeleteTopTable,
	}
	if cfg.TargetTable == "" && cfg.Uniqueness == nil && cfg.DeleteTopTable == "" {
		return []byte("{}")
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// decodeActionConfig relee la configuración extendida de una acción desde el
// JSONB `config`. Es tolerante: JSON inválido, nil o vacío devuelven ceros sin
// panic (espejo de decodeFieldSpecs). Inverso de encodeActionConfig.
func decodeActionConfig(b []byte) (targetTable string, uniqueness *UniquenessCheck, deleteTopTable string) {
	if len(b) == 0 {
		return "", nil, ""
	}
	var cfg actionConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", nil, ""
	}
	return cfg.TargetTable, cfg.Uniqueness, cfg.DeleteTopTable
}

func encodeWhitelist(w map[string][]string) []byte {
	if len(w) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(w)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func decodeWhitelist(b []byte) map[string][]string {
	if len(b) == 0 {
		return map[string][]string{}
	}
	var w map[string][]string
	if err := json.Unmarshal(b, &w); err != nil || w == nil {
		return map[string][]string{}
	}
	return w
}

func encodeConditions(c []ListCondition) []byte {
	if len(c) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func decodeConditions(b []byte) []ListCondition {
	if len(b) == 0 {
		return nil
	}
	var c []ListCondition
	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	return c
}
