// Command migrate-connections es el backfill one-shot que convierte los módulos
// PostgreSQL existentes (DSN directo cifrado en module_data_sources.secrets_enc)
// al modelo de plantillas de conexión (connection-templates), de forma
// idempotente y reversible (Req. 7). NO se ejecuta en el arranque del servidor:
// el usuario lo corre manualmente tras aplicar la migración 000008.
//
// Para cada módulo pendiente (source_type='postgres', secrets_enc no nulo,
// connection_template_id NULL), en UNA transacción propia:
//  1. descifra el DSN legado en memoria (referencia de verificación);
//  2. deriva una plantilla 1:1 (name mod-<slug>, todos los campos FIJOS);
//  3. copia secrets_enc -> secrets_enc_legacy (respaldo OBLIGATORIO, antes de
//     reescribir nada);
//  4. verifica equivalencia por COMPONENTES (no por string) entre el DSN legado
//     y el DSN efectivo ensamblado desde la plantilla;
//  5. solo si verifica, fija connection_template_id y secrets_enc = Seal('{}')
//     y hace COMMIT; en cualquier fallo hace ROLLBACK y lo reporta.
//
// El subcomando `--rollback` restaura secrets_enc desde el respaldo, suelta la
// FK (antes de borrar, por ON DELETE RESTRICT) y borra las plantillas mod-%.
//
// La lógica central (migrateDataSource / equivalencia) se factoriza en funciones
// puras testeables (espejo de cmd/seed).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"

	"gorm.io/gorm"

	"admin-backend/internal/config"
	"admin-backend/internal/connections"
	"admin-backend/internal/db"
	"admin-backend/internal/secrets"
)

// legacySecrets es el bundle legado tal como lo guardaba module-builder antes de
// esta feature: un único DSN directo cifrado.
type legacySecrets struct {
	DSN string `json:"dsn"`
}

// migrateResult es el desenlace de migrar (o intentar migrar) un módulo.
type migrateResult struct {
	Slug          string
	TemplateName  string
	Migrated      bool
	ManualPending bool
	Reason        string
}

// dataSourceRow es la vista mínima de module_data_sources que el backfill lee y
// escribe (no reutiliza la del paquete modules para no acoplar el comando).
type dataSourceRow struct {
	ID                   string  `gorm:"column:id"`
	ModuleID             string  `gorm:"column:module_id"`
	SourceType           string  `gorm:"column:source_type"`
	SecretsEnc           []byte  `gorm:"column:secrets_enc"`
	SecretsEncLegacy     []byte  `gorm:"column:secrets_enc_legacy"`
	ConnectionTemplateID *string `gorm:"column:connection_template_id"`
}

func (dataSourceRow) TableName() string { return "module_data_sources" }

// migrateDataSource es la FUNCIÓN PURA del backfill: descifra el DSN legado,
// deriva la plantilla 1:1 y verifica equivalencia. No toca la BD. Devuelve la
// estructura derivada + el bundle de valores FIJOS + el sellado del bundle FIJO,
// o un error/ManualPending cuando el DSN no es descomponible o no verifica.
func migrateDataSource(sealer *secrets.Sealer, legacyEnc []byte) (connections.DerivedTemplate, error) {
	plain, err := sealer.Open(legacyEnc)
	if err != nil {
		return connections.DerivedTemplate{}, fmt.Errorf("no se pudo descifrar el DSN legado")
	}
	var ls legacySecrets
	if err := json.Unmarshal(plain, &ls); err != nil {
		return connections.DerivedTemplate{}, fmt.Errorf("DSN legado con formato inválido")
	}
	derived, err := connections.DeriveTemplateFromLegacyDSN(ls.DSN)
	if err != nil {
		return connections.DerivedTemplate{}, fmt.Errorf("DSN no descomponible (dbname ausente o inválido): migración manual pendiente")
	}
	// Verificación de equivalencia por componentes: ensamblar el DSN efectivo y
	// comparar con el legado.
	effective, err := connections.AssembleDSN(derived.Fields, derived.Fixed, nil)
	if err != nil {
		return connections.DerivedTemplate{}, fmt.Errorf("no se pudo ensamblar el DSN efectivo: migración manual pendiente")
	}
	if !connections.LegacyEquivalent(ls.DSN, effective) {
		return connections.DerivedTemplate{}, fmt.Errorf("el DSN efectivo no es equivalente al legado: migración manual pendiente")
	}
	return derived, nil
}

// runBackfill procesa todos los módulos pendientes. Cada módulo va en su propia
// transacción; un fallo deja ese módulo intacto y continúa con el resto.
func runBackfill(gormDB *gorm.DB, sealer *secrets.Sealer) ([]migrateResult, error) {
	type pending struct {
		DSID       string `gorm:"column:ds_id"`
		Slug       string `gorm:"column:slug"`
		SecretsEnc []byte `gorm:"column:secrets_enc"`
	}
	var rows []pending
	err := gormDB.
		Table("module_data_sources AS ds").
		Select("ds.id AS ds_id, md.slug AS slug, ds.secrets_enc AS secrets_enc").
		Joins("JOIN module_definitions AS md ON md.id = ds.module_id").
		Where("ds.source_type = ? AND ds.secrets_enc IS NOT NULL AND ds.connection_template_id IS NULL", "postgres").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("migrate-connections: error listando módulos pendientes: %w", err)
	}

	results := make([]migrateResult, 0, len(rows))
	for _, p := range rows {
		res := migrateResult{Slug: p.Slug, TemplateName: "mod-" + p.Slug}
		derived, derr := migrateDataSource(sealer, p.SecretsEnc)
		if derr != nil {
			res.ManualPending = true
			res.Reason = derr.Error()
			results = append(results, res)
			continue
		}

		txErr := gormDB.Transaction(func(tx *gorm.DB) error {
			// Guardia de idempotencia dentro de la transacción: si ya tiene FK,
			// otro proceso lo migró; saltar.
			var current dataSourceRow
			if err := tx.Where("id = ?", p.DSID).First(&current).Error; err != nil {
				return err
			}
			if current.ConnectionTemplateID != nil && *current.ConnectionTemplateID != "" {
				return errAlreadyMigrated
			}

			// Sellar el bundle de valores FIJOS de la plantilla.
			fixedPlain, err := json.Marshal(derived.Fixed)
			if err != nil {
				return err
			}
			fixedSealed, err := sealer.Seal(fixedPlain)
			if err != nil {
				return err
			}
			fieldsJSON, err := json.Marshal(derived.Fields)
			if err != nil {
				return err
			}

			// Crear la plantilla mod-<slug>.
			tmpl := templateInsert{
				Name:       res.TemplateName,
				Fields:     fieldsJSON,
				SecretsEnc: fixedSealed,
			}
			if err := tx.Create(&tmpl).Error; err != nil {
				return err
			}

			// Copiar el legado al respaldo ANTES de reescribir nada.
			if err := tx.Model(&dataSourceRow{}).Where("id = ?", p.DSID).
				Update("secrets_enc_legacy", gorm.Expr("secrets_enc")).Error; err != nil {
				return err
			}

			// Bundle VARIABLE vacío (mapeo 1:1: todos los campos son fijos).
			emptyVar, err := sealer.Seal([]byte(`{"variableValues":{}}`))
			if err != nil {
				return err
			}

			// Fijar la FK y reescribir secrets_enc = Seal('{}') SOLO tras verificar.
			if err := tx.Model(&dataSourceRow{}).Where("id = ?", p.DSID).Updates(map[string]any{
				"connection_template_id": tmpl.ID,
				"secrets_enc":            emptyVar,
			}).Error; err != nil {
				return err
			}
			return nil
		})

		if txErr != nil {
			if errors.Is(txErr, errAlreadyMigrated) {
				res.Reason = "ya migrado (idempotente): sin cambios"
				results = append(results, res)
				continue
			}
			res.ManualPending = true
			res.Reason = "error en la transacción: " + txErr.Error()
			results = append(results, res)
			continue
		}
		res.Migrated = true
		results = append(results, res)
	}
	return results, nil
}

// errAlreadyMigrated marca el salto idempotente dentro de la transacción.
var errAlreadyMigrated = errors.New("módulo ya migrado")

// templateInsert es la fila mínima para crear una plantilla desde el backfill.
type templateInsert struct {
	ID         string `gorm:"column:id;default:gen_random_uuid()"`
	Name       string `gorm:"column:name"`
	Fields     []byte `gorm:"column:fields"`
	SecretsEnc []byte `gorm:"column:secrets_enc"`
}

func (templateInsert) TableName() string { return "connection_templates" }

// runRollback deshace el backfill en ORDEN (restaurar datos y soltar la FK antes
// de borrar plantillas, por ON DELETE RESTRICT), en una transacción.
func runRollback(gormDB *gorm.DB) error {
	return gormDB.Transaction(func(tx *gorm.DB) error {
		// 1) Restaurar el DSN legado y soltar la FK.
		if err := tx.Exec(
			`UPDATE module_data_sources
			    SET secrets_enc = secrets_enc_legacy,
			        connection_template_id = NULL
			  WHERE connection_template_id IS NOT NULL
			    AND secrets_enc_legacy IS NOT NULL`).Error; err != nil {
			return fmt.Errorf("migrate-connections: error restaurando el DSN legado: %w", err)
		}
		// 2) Borrar las plantillas creadas por el backfill (ya sin referencias).
		if err := tx.Exec(`DELETE FROM connection_templates WHERE name LIKE 'mod-%'`).Error; err != nil {
			return fmt.Errorf("migrate-connections: error borrando plantillas del backfill: %w", err)
		}
		return nil
	})
}

func main() {
	rollback := flag.Bool("rollback", false, "revierte el backfill (restaura DSN legado y borra plantillas mod-%)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("migrate-connections: error cargando la configuración: %v", err)
	}
	if len(cfg.ModuleSecretsKey) == 0 {
		log.Fatalf("migrate-connections: MODULE_SECRETS_KEY ausente; no se puede descifrar ni cifrar")
	}
	sealer, err := secrets.New(cfg.ModuleSecretsKey)
	if err != nil {
		log.Fatalf("migrate-connections: MODULE_SECRETS_KEY inválida: %v", err)
	}

	gormDB, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("migrate-connections: error abriendo la conexión: %v", err)
	}
	defer func() {
		if cerr := db.Close(gormDB); cerr != nil {
			log.Printf("migrate-connections: advertencia al cerrar la conexión: %v", cerr)
		}
	}()

	if *rollback {
		if err := runRollback(gormDB); err != nil {
			log.Fatalf("migrate-connections: %v", err)
		}
		log.Printf("migrate-connections: rollback completado (DSN legado restaurado, plantillas mod-%% borradas)")
		return
	}

	results, err := runBackfill(gormDB, sealer)
	if err != nil {
		log.Fatalf("migrate-connections: %v", err)
	}

	migrated, pending := 0, 0
	for _, r := range results {
		switch {
		case r.Migrated:
			migrated++
			log.Printf("migrate-connections: módulo %q migrado -> plantilla %q", r.Slug, r.TemplateName)
		case r.ManualPending:
			pending++
			log.Printf("migrate-connections: módulo %q MIGRACIÓN MANUAL PENDIENTE: %s", r.Slug, r.Reason)
		default:
			log.Printf("migrate-connections: módulo %q omitido: %s", r.Slug, r.Reason)
		}
	}
	log.Printf("migrate-connections: resumen -> %d migrados, %d manuales pendientes, %d total", migrated, pending, len(results))
	log.Printf("migrate-connections: para revertir el backfill ejecuta este comando con --rollback ANTES de aplicar 000008 down.sql")
}
