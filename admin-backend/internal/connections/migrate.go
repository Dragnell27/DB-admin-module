package connections

// Esta superficie exportada es la API que consume el comando one-shot de
// backfill (cmd/migrate-connections). Mantiene las funciones de parseo y
// equivalencia del DSN dentro de este paquete (junto al dominio de plantillas),
// respetando la invariante de dependencias (modules/cmd -> connections).

// DerivedTemplate es el resultado de derivar una plantilla 1:1 desde un DSN
// legado: su estructura (todos los campos FIJOS) y el bundle de valores FIJOS en
// claro (que el llamador cifra con el Sealer antes de persistir).
type DerivedTemplate struct {
	Fields []TemplateField
	Fixed  FixedValues
}

// DeriveTemplateFromLegacyDSN parsea un DSN legado (acepta `postgres://...` y
// formato libpq `clave=valor`) y deriva una plantilla 1:1 con todos sus
// componentes marcados FIJO. Devuelve ErrValidation (envuelto) si el DSN está
// vacío o no tiene `dbname` (undecomponible -> migración manual pendiente). Es
// una función pura.
func DeriveTemplateFromLegacyDSN(dsn string) (DerivedTemplate, error) {
	comp, err := parseLegacyDSN(dsn)
	if err != nil {
		return DerivedTemplate{}, err
	}
	if comp.DBName == "" {
		return DerivedTemplate{}, ErrValidation
	}
	fields, fixed := fieldsFromComponents(comp)
	return DerivedTemplate{Fields: fields, Fixed: fixed}, nil
}

// LegacyEquivalent indica si el DSN efectivo ensamblado desde una plantilla
// derivada es equivalente (por componentes normalizados) al DSN legado original.
// Compara host (minúsculas), port (default 5432), user/password/dbname exactos,
// sslmode (ausente==prefer) y options por conjunto EXACTO con nombre original.
// Es una función pura; nunca loguea valores.
func LegacyEquivalent(legacyDSN, effectiveDSN string) bool {
	a, err := parseLegacyDSN(legacyDSN)
	if err != nil {
		return false
	}
	b, err := parseLegacyDSN(effectiveDSN)
	if err != nil {
		return false
	}
	return dsnEquivalent(a, b)
}
