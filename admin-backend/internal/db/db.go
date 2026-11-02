// Package db gestiona la conexión a PostgreSQL mediante GORM para el
// admin-backend. Construye el DSN a partir de la configuración (sin valores
// sensibles embebidos; todo proviene del entorno vía internal/config) y expone
// una función de apertura reutilizable por los handlers y por el seed
// (Req. 21.5, 23.3).
package db

import (
	"fmt"
	"time"

	"admin-backend/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Valores por defecto del pool de conexiones. Son sensatos para un BFF que
// atiende tráfico moderado y pueden ajustarse si el perfil de carga cambia.
const (
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = time.Hour
	defaultConnMaxIdleTime = 10 * time.Minute
)

// Options permite afinar el comportamiento de la conexión. El valor cero es
// válido y aplica los valores por defecto del pool.
type Options struct {
	// MaxOpenConns limita el número máximo de conexiones abiertas al pool.
	MaxOpenConns int
	// MaxIdleConns limita el número máximo de conexiones inactivas en el pool.
	MaxIdleConns int
	// ConnMaxLifetime es el tiempo máximo que una conexión puede reutilizarse.
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime es el tiempo máximo que una conexión puede permanecer
	// inactiva antes de cerrarse.
	ConnMaxIdleTime time.Duration
	// GormLogLevel controla la verbosidad del logger de GORM. Si es cero se usa
	// logger.Warn.
	GormLogLevel logger.LogLevel
}

// DSN construye la cadena de conexión de PostgreSQL a partir de la
// configuración. No incluye credenciales embebidas: todos los valores provienen
// de config.Config (poblado desde el entorno).
func DSN(cfg *config.Config) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBSSLMode,
	)
}

// Open abre una conexión GORM a PostgreSQL usando los valores por defecto del
// pool. Es la función reutilizable que consumen los handlers y el seed.
func Open(cfg *config.Config) (*gorm.DB, error) {
	return OpenWithOptions(cfg, Options{})
}

// OpenWithOptions abre una conexión GORM a PostgreSQL aplicando las opciones de
// pool indicadas (o los valores por defecto cuando un campo es cero). Verifica
// la conectividad con un ping antes de devolver la conexión.
func OpenWithOptions(cfg *config.Config, opts Options) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("db: la configuración no puede ser nil")
	}

	logLevel := opts.GormLogLevel
	if logLevel == 0 {
		logLevel = logger.Warn
	}

	gormDB, err := gorm.Open(postgres.Open(DSN(cfg)), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("db: no se pudo abrir la conexión a PostgreSQL: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db: no se pudo obtener el *sql.DB subyacente: %w", err)
	}

	sqlDB.SetMaxOpenConns(firstPositive(opts.MaxOpenConns, defaultMaxOpenConns))
	sqlDB.SetMaxIdleConns(firstPositive(opts.MaxIdleConns, defaultMaxIdleConns))
	sqlDB.SetConnMaxLifetime(firstPositiveDuration(opts.ConnMaxLifetime, defaultConnMaxLifetime))
	sqlDB.SetConnMaxIdleTime(firstPositiveDuration(opts.ConnMaxIdleTime, defaultConnMaxIdleTime))

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db: no se pudo establecer conexión con PostgreSQL (ping): %w", err)
	}

	return gormDB, nil
}

// Close cierra la conexión subyacente de una instancia GORM. Es seguro llamarla
// con un *gorm.DB nil.
func Close(gormDB *gorm.DB) error {
	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("db: no se pudo obtener el *sql.DB subyacente para cerrar: %w", err)
	}
	return sqlDB.Close()
}

// firstPositive devuelve v si es positivo, de lo contrario def.
func firstPositive(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

// firstPositiveDuration devuelve v si es positivo, de lo contrario def.
func firstPositiveDuration(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}
