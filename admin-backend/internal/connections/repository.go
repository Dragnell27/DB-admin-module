package connections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"admin-backend/internal/secrets"
)

// Repository define la persistencia de las plantillas de conexión. Los valores
// FIJOS se cifran al persistir (bundle completo) y nunca se devuelven en las
// vistas públicas (Template).
type Repository interface {
	// List devuelve todas las plantillas (vista pública, con UsedByModules).
	List(ctx context.Context) ([]Template, error)
	// Get devuelve una plantilla por id (vista pública, con UsedByModules).
	Get(ctx context.Context, id string) (*Template, error)
	// Create inserta una plantilla nueva, cifrando los valores FIJOS.
	Create(ctx context.Context, name string, fields []TemplateField, fixed FixedValues) (*Template, error)
	// Update reemplaza la estructura y hace merge por clave de los valores FIJOS
	// (vacío/ausente = conservar; presente = reemplazar).
	Update(ctx context.Context, id, name string, fields []TemplateField, fixed FixedValues) (*Template, error)
	// Delete elimina la plantilla; ErrTemplateInUse si está referenciada.
	Delete(ctx context.Context, id string) error
	// SlugsUsingTemplate devuelve los slugs de módulos que referencian la
	// plantilla (para invalidar sus pools tras una edición).
	SlugsUsingTemplate(ctx context.Context, id string) ([]string, error)
	// TemplateExists indica si existe una plantilla con el id dado. Lo usa el
	// repositorio de módulos para validar la FK sin importar este paquete.
	TemplateExists(ctx context.Context, id string) (bool, error)
	// VariableKeys devuelve las claves VARIABLE de una plantilla (para el merge
	// por clave de los valores variables de un módulo).
	VariableKeys(ctx context.Context, id string) ([]string, error)
	// RevealFixedValues descifra y devuelve los valores FIJOS de una plantilla y
	// su estructura, SOLO para el ensamblado en memoria del DSN efectivo. Nunca
	// se expone al frontend.
	RevealFixedValues(ctx context.Context, id string) ([]TemplateField, FixedValues, error)
}

// gormRepository implementa Repository sobre GORM, cifrando los valores FIJOS
// con un secrets.Sealer.
type gormRepository struct {
	db     *gorm.DB
	sealer *secrets.Sealer
}

// NewRepository crea un Repository sobre la conexión GORM dada y el sellador de
// secretos (internal/secrets) para cifrar/descifrar los valores FIJOS.
func NewRepository(db *gorm.DB, sealer *secrets.Sealer) Repository {
	return &gormRepository{db: db, sealer: sealer}
}

// List devuelve todas las plantillas con UsedByModules poblado en UNA consulta
// agregada (sin N+1).
func (r *gormRepository) List(ctx context.Context) ([]Template, error) {
	var rows []templateRow
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("connections: error listando plantillas: %w", err)
	}
	counts, err := r.usageCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(rows))
	for _, row := range rows {
		t := toTemplate(row)
		t.UsedByModules = counts[row.ID]
		out = append(out, t)
	}
	return out, nil
}

// Get devuelve una plantilla por id con el conteo real de módulos que la usan.
func (r *gormRepository) Get(ctx context.Context, id string) (*Template, error) {
	row, err := r.findRow(ctx, id)
	if err != nil {
		return nil, err
	}
	count, err := r.countUsing(ctx, id)
	if err != nil {
		return nil, err
	}
	t := toTemplate(*row)
	t.UsedByModules = count
	return &t, nil
}

// Create valida la estructura, cifra los valores FIJOS y persiste la plantilla.
func (r *gormRepository) Create(ctx context.Context, name string, fields []TemplateField, fixed FixedValues) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: el nombre es obligatorio", ErrValidation)
	}
	if err := validateFields(fields); err != nil {
		return nil, err
	}
	// En CREACIÓN no hay bundle previo: un FIJO sin valor se rechaza.
	merged, err := mergeFixedValues(fields, nil, fixed)
	if err != nil {
		return nil, err
	}
	sealed, err := r.sealFixed(merged)
	if err != nil {
		return nil, err
	}

	row := templateRow{
		Name:       name,
		Fields:     encodeFields(fields),
		SecretsEnc: sealed,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&templateRow{}).Where("name = ?", name).Count(&count).Error; err != nil {
			return fmt.Errorf("connections: error verificando nombre: %w", err)
		}
		if count > 0 {
			return ErrNameTaken
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("connections: error creando plantilla: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	t := toTemplate(row)
	return &t, nil
}

// Update reemplaza la estructura y hace merge por clave de los valores FIJOS
// contra el bundle previo descifrado.
func (r *gormRepository) Update(ctx context.Context, id, name string, fields []TemplateField, fixed FixedValues) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: el nombre es obligatorio", ErrValidation)
	}
	if err := validateFields(fields); err != nil {
		return nil, err
	}

	var result Template
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prev templateRow
		if err := tx.Where("id = ?", id).First(&prev).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("connections: error buscando plantilla a actualizar: %w", err)
		}

		// Nombre duplicado en OTRA plantilla.
		var count int64
		if err := tx.Model(&templateRow{}).Where("name = ? AND id <> ?", name, id).Count(&count).Error; err != nil {
			return fmt.Errorf("connections: error verificando nombre: %w", err)
		}
		if count > 0 {
			return ErrNameTaken
		}

		// Descifrar el bundle FIJO previo para el merge por clave.
		prevFixed, err := r.openFixed(prev.SecretsEnc)
		if err != nil {
			return err
		}
		merged, err := mergeFixedValues(fields, prevFixed, fixed)
		if err != nil {
			return err
		}
		sealed, err := r.sealFixed(merged)
		if err != nil {
			return err
		}

		updates := map[string]any{
			"name":        name,
			"fields":      encodeFields(fields),
			"secrets_enc": sealed,
		}
		if err := tx.Model(&templateRow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("connections: error actualizando plantilla: %w", err)
		}
		prev.Name = name
		prev.Fields = encodeFields(fields)
		prev.SecretsEnc = sealed
		result = toTemplate(prev)
		return nil
	}); err != nil {
		return nil, err
	}

	count, err := r.countUsing(ctx, id)
	if err != nil {
		return nil, err
	}
	result.UsedByModules = count
	return &result, nil
}

// Delete elimina la plantilla; si está referenciada por módulos, ErrTemplateInUse.
func (r *gormRepository) Delete(ctx context.Context, id string) error {
	count, err := r.countUsing(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrTemplateInUse
	}
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&templateRow{})
	if res.Error != nil {
		// La FK ON DELETE RESTRICT es la red de seguridad final.
		return fmt.Errorf("connections: error eliminando plantilla: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SlugsUsingTemplate devuelve los slugs de módulos que referencian la plantilla.
func (r *gormRepository) SlugsUsingTemplate(ctx context.Context, id string) ([]string, error) {
	var slugs []string
	err := r.db.WithContext(ctx).
		Table("module_data_sources AS ds").
		Joins("JOIN module_definitions AS md ON md.id = ds.module_id").
		Where("ds.connection_template_id = ?", id).
		Pluck("md.slug", &slugs).Error
	if err != nil {
		return nil, fmt.Errorf("connections: error listando módulos de la plantilla: %w", err)
	}
	return slugs, nil
}

// TemplateExists indica si la plantilla existe.
func (r *gormRepository) TemplateExists(ctx context.Context, id string) (bool, error) {
	if strings.TrimSpace(id) == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&templateRow{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("connections: error verificando plantilla: %w", err)
	}
	return count > 0, nil
}

// VariableKeys devuelve las claves VARIABLE de la plantilla.
func (r *gormRepository) VariableKeys(ctx context.Context, id string) ([]string, error) {
	row, err := r.findRow(ctx, id)
	if err != nil {
		return nil, err
	}
	fields := decodeFields(row.Fields)
	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Kind == FieldVariable {
			keys = append(keys, f.Key)
		}
	}
	return keys, nil
}

// RevealFixedValues descifra los valores FIJOS y devuelve la estructura, solo
// para el ensamblado en memoria del DSN efectivo.
func (r *gormRepository) RevealFixedValues(ctx context.Context, id string) ([]TemplateField, FixedValues, error) {
	row, err := r.findRow(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	fixed, err := r.openFixed(row.SecretsEnc)
	if err != nil {
		return nil, nil, err
	}
	return decodeFields(row.Fields), fixed, nil
}

// --- helpers internos -------------------------------------------------------

func (r *gormRepository) findRow(ctx context.Context, id string) (*templateRow, error) {
	var row templateRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("connections: error buscando plantilla: %w", err)
	}
	return &row, nil
}

// countUsing cuenta los módulos que referencian la plantilla.
func (r *gormRepository) countUsing(ctx context.Context, id string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("module_data_sources").
		Where("connection_template_id = ?", id).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("connections: error contando módulos de la plantilla: %w", err)
	}
	return int(count), nil
}

// usageCounts devuelve el conteo de módulos por plantilla en UNA consulta
// agregada (sin N+1).
func (r *gormRepository) usageCounts(ctx context.Context) (map[string]int, error) {
	type aggRow struct {
		ConnectionTemplateID string
		N                    int64
	}
	var rows []aggRow
	err := r.db.WithContext(ctx).
		Table("module_data_sources").
		Select("connection_template_id, COUNT(*) AS n").
		Where("connection_template_id IS NOT NULL").
		Group("connection_template_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("connections: error agregando uso de plantillas: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ConnectionTemplateID] = int(row.N)
	}
	return out, nil
}

// sealFixed serializa y cifra el bundle de valores FIJOS. Devuelve nil si está
// vacío (plantilla sin valores fijos, p. ej. todos los campos VARIABLE).
func (r *gormRepository) sealFixed(fixed FixedValues) ([]byte, error) {
	if len(fixed) == 0 {
		return nil, nil
	}
	plain, err := json.Marshal(fixed)
	if err != nil {
		return nil, fmt.Errorf("connections: no se pudieron serializar los valores fijos: %w", err)
	}
	sealed, err := r.sealer.Seal(plain)
	if err != nil {
		return nil, fmt.Errorf("connections: no se pudieron cifrar los valores fijos: %w", err)
	}
	return sealed, nil
}

// openFixed descifra el bundle de valores FIJOS. Bundle vacío -> mapa vacío.
func (r *gormRepository) openFixed(enc []byte) (FixedValues, error) {
	if len(enc) == 0 {
		return FixedValues{}, nil
	}
	plain, err := r.sealer.Open(enc)
	if err != nil {
		return nil, fmt.Errorf("connections: no se pudieron descifrar los valores fijos: %w", err)
	}
	var fixed FixedValues
	if err := json.Unmarshal(plain, &fixed); err != nil {
		return nil, fmt.Errorf("connections: valores fijos con formato inválido: %w", err)
	}
	if fixed == nil {
		fixed = FixedValues{}
	}
	return fixed, nil
}

// toTemplate convierte una fila a la vista pública (sin valores fijos).
func toTemplate(row templateRow) Template {
	return Template{
		ID:             row.ID,
		Name:           row.Name,
		Fields:         decodeFields(row.Fields),
		HasFixedValues: len(row.SecretsEnc) > 0,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

// validateFields valida la estructura de la plantilla (Req. 1.6): al menos un
// campo, sin claves duplicadas, role válido.
func validateFields(fields []TemplateField) error {
	if len(fields) == 0 {
		return fmt.Errorf("%w: la plantilla debe tener al menos un campo", ErrValidation)
	}
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		key := strings.TrimSpace(f.Key)
		if key == "" {
			return fmt.Errorf("%w: hay un campo con clave vacía", ErrValidation)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%w: clave duplicada %q", ErrValidation, key)
		}
		seen[key] = struct{}{}
		if f.Kind != FieldFixed && f.Kind != FieldVariable {
			return fmt.Errorf("%w: el campo %q tiene una marca inválida %q", ErrValidation, key, f.Kind)
		}
		if !IsValidRole(f.Role) {
			return fmt.Errorf("%w: el campo %q tiene un role inválido %q", ErrValidation, key, f.Role)
		}
	}
	return nil
}

// mergeFixedValues aplica, por cada clave FIJA declarada en fields, la regla de
// merge contra el bundle previo descifrado (nil en creación):
//   - En CREACIÓN (prev nil): FIJO entrante vacío -> ErrValidation.
//   - En EDICIÓN: FIJO no presente en prev y entrante vacío -> ErrValidation;
//     FIJO presente en prev y entrante vacío -> conservar prev[key]; con valor ->
//     usar el nuevo.
//
// Las claves de prev que ya no estén declaradas como FIJAS se descartan. Es una
// función pura.
func mergeFixedValues(fields []TemplateField, prev, incoming FixedValues) (FixedValues, error) {
	isCreate := prev == nil
	merged := FixedValues{}
	for _, f := range fields {
		if f.Kind != FieldFixed {
			continue
		}
		key := strings.TrimSpace(f.Key)
		newVal := strings.TrimSpace(incoming[key])
		if newVal != "" {
			merged[key] = newVal
			continue
		}
		// Entrante vacío.
		if isCreate {
			return nil, fmt.Errorf("%w: el campo fijo %q requiere un valor", ErrValidation, key)
		}
		prevVal, existed := prev[key]
		if !existed {
			return nil, fmt.Errorf("%w: el campo fijo nuevo %q requiere un valor", ErrValidation, key)
		}
		merged[key] = prevVal
	}
	return merged, nil
}
