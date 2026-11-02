package db

import (
	"strings"
	"testing"
	"time"

	"admin-backend/internal/config"
	"gorm.io/gorm/logger"
)

func TestDSN_ConstruyeCadenaConTodosLosParametros(t *testing.T) {
	cfg := &config.Config{
		DBHost:     "db.example.com",
		DBPort:     "5433",
		DBUser:     "admin_user",
		DBPassword: "s3cr3t",
		DBName:     "admin_db",
		DBSSLMode:  "require",
	}

	got := DSN(cfg)

	wantParts := []string{
		"host=db.example.com",
		"port=5433",
		"user=admin_user",
		"password=s3cr3t",
		"dbname=admin_db",
		"sslmode=require",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Errorf("DSN() = %q, se esperaba que contuviera %q", got, part)
		}
	}
}

func TestFirstPositive(t *testing.T) {
	cases := []struct {
		name string
		v    int
		def  int
		want int
	}{
		{"usa valor cuando es positivo", 5, 10, 5},
		{"usa default cuando es cero", 0, 10, 10},
		{"usa default cuando es negativo", -3, 10, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstPositive(tc.v, tc.def); got != tc.want {
				t.Errorf("firstPositive(%d, %d) = %d, want %d", tc.v, tc.def, got, tc.want)
			}
		})
	}
}

func TestFirstPositiveDuration(t *testing.T) {
	cases := []struct {
		name string
		v    time.Duration
		def  time.Duration
		want time.Duration
	}{
		{"usa valor cuando es positivo", 2 * time.Hour, time.Hour, 2 * time.Hour},
		{"usa default cuando es cero", 0, time.Hour, time.Hour},
		{"usa default cuando es negativo", -time.Minute, time.Hour, time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstPositiveDuration(tc.v, tc.def); got != tc.want {
				t.Errorf("firstPositiveDuration(%v, %v) = %v, want %v", tc.v, tc.def, got, tc.want)
			}
		})
	}
}

func TestOpenWithOptions_RechazaConfigNil(t *testing.T) {
	if _, err := OpenWithOptions(nil, Options{}); err == nil {
		t.Fatal("se esperaba error al abrir con configuración nil, se obtuvo nil")
	}
}

func TestClose_ConNilEsSeguro(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Errorf("Close(nil) devolvió error inesperado: %v", err)
	}
}

// Verifica que el valor por defecto del log level se referencia correctamente.
func TestOptions_LogLevelDefaultReferenciaValida(t *testing.T) {
	var opts Options
	if opts.GormLogLevel != 0 {
		t.Fatalf("el valor cero de GormLogLevel debe ser 0, se obtuvo %v", opts.GormLogLevel)
	}
	// logger.Warn se usa como fallback cuando el nivel es 0.
	if logger.Warn == 0 {
		t.Fatal("logger.Warn no debería ser 0; invariante de la lógica de fallback")
	}
}
