package db

// Este archivo implementa un runner de migraciones mínimo que aplica, al
// arrancar, los archivos SQL `*.up.sql` embebidos en el paquete migrations
// (design.md "Capa de datos", tarea 2.2: "Aplicar migraciones al arrancar si el
// esquema no está presente"). No se añade golang-migrate como dependencia: el
// runner lee los archivos embebidos, lleva el control de las versiones ya
// aplicadas en una tabla `schema_migrations` y ejecuta cada migración una sola
// vez dentro de una transacción.
//
// Las migraciones del proyecto son idempotentes a nivel SQL (usan IF NOT
// EXISTS), pero el registro en `schema_migrations` evita reejecutarlas en cada
// arranque y deja trazabilidad de qué versiones se aplicaron.

import (
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"admin-backend/migrations"
)

// upSuffix es el sufijo que identifica una migración de avance ("up").
const upSuffix = ".up.sql"

// migrationFile representa un archivo de migración "up" con su versión numérica
// (derivada del prefijo del nombre, p. ej. 000001_create_users.up.sql → 1), su
// nombre y su contenido SQL.
type migrationFile struct {
	version int64
	name    string
	sql     string
}

// Migrate aplica las migraciones "up" pendientes sobre la conexión dada, en
// orden ascendente de versión. Crea la tabla de control `schema_migrations` si
// no existe, omite las versiones ya registradas y ejecuta cada migración nueva
// dentro de una transacción, registrándola al terminar. Es seguro invocarla en
// cada arranque: si no hay migraciones pendientes, no realiza cambios.
func Migrate(gormDB *gorm.DB) error {
	if gormDB == nil {
		return fmt.Errorf("db: la conexión no puede ser nil para migrar")
	}

	files, err := loadUpMigrations()
	if err != nil {
		return err
	}

	if err := ensureMigrationsTable(gormDB); err != nil {
		return err
	}

	for _, m := range files {
		applied, err := migrationApplied(gormDB, m.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := applyMigration(gormDB, m); err != nil {
			return fmt.Errorf("db: fallo aplicando la migración %q: %w", m.name, err)
		}
	}

	return nil
}

// loadUpMigrations lee del FS embebido todos los archivos `*.up.sql`, parsea su
// versión del prefijo del nombre y los devuelve ordenados ascendentemente por
// versión.
func loadUpMigrations() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("db: no se pudieron listar las migraciones embebidas: %w", err)
	}

	var result []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), upSuffix) {
			continue
		}

		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}

		content, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("db: no se pudo leer la migración %q: %w", entry.Name(), err)
		}

		result = append(result, migrationFile{
			version: version,
			name:    entry.Name(),
			sql:     string(content),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].version < result[j].version
	})

	return result, nil
}

// parseMigrationVersion extrae la versión numérica del prefijo del nombre de
// archivo con la convención golang-migrate ("000001_...": todo lo anterior al
// primer '_').
func parseMigrationVersion(name string) (int64, error) {
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return 0, fmt.Errorf("db: nombre de migración inválido %q (se esperaba prefijo numérico seguido de '_')", name)
	}
	version, err := strconv.ParseInt(name[:idx], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("db: versión no numérica en la migración %q: %w", name, err)
	}
	return version, nil
}

// ensureMigrationsTable crea la tabla de control si no existe.
func ensureMigrationsTable(gormDB *gorm.DB) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`
	if err := gormDB.Exec(ddl).Error; err != nil {
		return fmt.Errorf("db: no se pudo asegurar la tabla schema_migrations: %w", err)
	}
	return nil
}

// migrationApplied indica si la versión dada ya está registrada como aplicada.
func migrationApplied(gormDB *gorm.DB, version int64) (bool, error) {
	var count int64
	err := gormDB.
		Table("schema_migrations").
		Where("version = ?", version).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("db: no se pudo consultar schema_migrations: %w", err)
	}
	return count > 0, nil
}

// applyMigration ejecuta el SQL de una migración y registra su versión dentro
// de una única transacción, de modo que una migración a medias no quede
// marcada como aplicada.
func applyMigration(gormDB *gorm.DB, m migrationFile) error {
	return gormDB.Transaction(func(tx *gorm.DB) error {
		if strings.TrimSpace(m.sql) != "" {
			if err := tx.Exec(m.sql).Error; err != nil {
				return err
			}
		}
		return tx.Exec(
			"INSERT INTO schema_migrations (version) VALUES (?)",
			m.version,
		).Error
	})
}
