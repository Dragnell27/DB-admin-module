package config

import (
	"strings"
	"testing"
)

// TestLoadRejectsUnsupportedActiveCountry verifica que el arranque falla cuando
// OTT_ACTIVE_COUNTRY está ausente o fuera del conjunto permitido
// {gt,sv,ni,hn,cr} (Req. 3.1).
func TestLoadRejectsUnsupportedActiveCountry(t *testing.T) {
	tests := []struct {
		name          string
		activeCountry string
	}{
		{name: "vacío", activeCountry: ""},
		{name: "solo espacios", activeCountry: "   "},
		{name: "país no soportado us", activeCountry: "us"},
		{name: "país no soportado xx", activeCountry: "xx"},
		{name: "cadena arbitraria", activeCountry: "guatemala"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTT_ACTIVE_COUNTRY", tc.activeCountry)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() debía fallar con OTT_ACTIVE_COUNTRY=%q, pero no devolvió error (cfg=%+v)", tc.activeCountry, cfg)
			}
			if !strings.Contains(err.Error(), "OTT_ACTIVE_COUNTRY") {
				t.Errorf("el error debía mencionar OTT_ACTIVE_COUNTRY, se obtuvo: %v", err)
			}
		})
	}
}

// TestLoadSucceedsForEachSupportedCountry verifica que el arranque funciona para
// cada país soportado y que OTT_ACTIVE_COUNTRY se normaliza (minúsculas y
// recorte de espacios) (Req. 3.1).
func TestLoadSucceedsForEachSupportedCountry(t *testing.T) {
	for _, country := range SupportedCountries {
		t.Run(country, func(t *testing.T) {
			t.Setenv("OTT_ACTIVE_COUNTRY", country)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() no debía fallar para país soportado %q: %v", country, err)
			}
			if cfg.OTTActiveCountry != country {
				t.Errorf("OTTActiveCountry = %q, se esperaba %q", cfg.OTTActiveCountry, country)
			}
		})
	}
}

// TestLoadNormalizesActiveCountry verifica que un valor con mayúsculas y espacios
// como " GT " se normaliza a "gt" y permite un arranque válido (Req. 3.1).
func TestLoadNormalizesActiveCountry(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "mayúsculas con espacios", input: " GT ", want: "gt"},
		{name: "mayúsculas", input: "SV", want: "sv"},
		{name: "mezcla con tabulación", input: "\tCr\t", want: "cr"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTT_ACTIVE_COUNTRY", tc.input)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() no debía fallar con entrada %q: %v", tc.input, err)
			}
			if cfg.OTTActiveCountry != tc.want {
				t.Errorf("OTTActiveCountry = %q, se esperaba %q (entrada %q)", cfg.OTTActiveCountry, tc.want, tc.input)
			}
		})
	}
}

// TestLoadReadsOTTCredentialsFromEnv verifica que las credenciales OTT se leen
// por país desde el entorno y no están embebidas en el código: al definir las
// variables OTT_<PAIS>_CLIENT_ID / OTT_<PAIS>_CLIENT_SECRET, esos valores deben
// aparecer en OTTCredentialsByCountry (Req. 23.3).
func TestLoadReadsOTTCredentialsFromEnv(t *testing.T) {
	t.Setenv("OTT_ACTIVE_COUNTRY", "gt")

	// Definir credenciales distintas por país para demostrar la lectura por país.
	for _, country := range SupportedCountries {
		upper := strings.ToUpper(country)
		t.Setenv("OTT_"+upper+"_CLIENT_ID", "client-id-"+country)
		t.Setenv("OTT_"+upper+"_CLIENT_SECRET", "client-secret-"+country)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() no debía fallar: %v", err)
	}

	if len(cfg.OTTCredentialsByCountry) != len(SupportedCountries) {
		t.Fatalf("OTTCredentialsByCountry tiene %d entradas, se esperaban %d",
			len(cfg.OTTCredentialsByCountry), len(SupportedCountries))
	}

	for _, country := range SupportedCountries {
		creds, ok := cfg.OTTCredentialsByCountry[country]
		if !ok {
			t.Errorf("falta la entrada de credenciales para el país %q", country)
			continue
		}
		wantID := "client-id-" + country
		wantSecret := "client-secret-" + country
		if creds.ClientID != wantID {
			t.Errorf("país %q: ClientID = %q, se esperaba %q (debe provenir del entorno)", country, creds.ClientID, wantID)
		}
		if creds.ClientSecret != wantSecret {
			t.Errorf("país %q: ClientSecret = %q, se esperaba %q (debe provenir del entorno)", country, creds.ClientSecret, wantSecret)
		}
	}
}

// TestLoadCredentialsEmptyWhenEnvUnset verifica que, sin variables de entorno,
// las credenciales OTT quedan vacías: la prueba demuestra que no hay secretos
// embebidos como valores por defecto en el código (Req. 23.3).
func TestLoadCredentialsEmptyWhenEnvUnset(t *testing.T) {
	t.Setenv("OTT_ACTIVE_COUNTRY", "gt")

	// Garantizar que no haya credenciales en el entorno para esta prueba.
	for _, country := range SupportedCountries {
		upper := strings.ToUpper(country)
		t.Setenv("OTT_"+upper+"_CLIENT_ID", "")
		t.Setenv("OTT_"+upper+"_CLIENT_SECRET", "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() no debía fallar: %v", err)
	}

	for _, country := range SupportedCountries {
		creds := cfg.OTTCredentialsByCountry[country]
		if creds.ClientID != "" || creds.ClientSecret != "" {
			t.Errorf("país %q: se esperaban credenciales vacías sin entorno, se obtuvo ClientID=%q ClientSecret=%q (posible secreto embebido)",
				country, creds.ClientID, creds.ClientSecret)
		}
	}
}

// TestLoadRejectsInvalidIntegers verifica que valores no enteros o no positivos
// en BCRYPT_COST / ADMIN_JWT_EXPIRES_MINUTES impiden un arranque seguro.
func TestLoadRejectsInvalidIntegers(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantInErr string
	}{
		{name: "bcrypt no entero", key: "BCRYPT_COST", value: "abc", wantInErr: "BCRYPT_COST"},
		{name: "bcrypt cero", key: "BCRYPT_COST", value: "0", wantInErr: "BCRYPT_COST"},
		{name: "bcrypt negativo", key: "BCRYPT_COST", value: "-1", wantInErr: "BCRYPT_COST"},
		{name: "jwt expires no entero", key: "ADMIN_JWT_EXPIRES_MINUTES", value: "x", wantInErr: "ADMIN_JWT_EXPIRES_MINUTES"},
		{name: "jwt expires cero", key: "ADMIN_JWT_EXPIRES_MINUTES", value: "0", wantInErr: "ADMIN_JWT_EXPIRES_MINUTES"},
		{name: "jwt expires negativo", key: "ADMIN_JWT_EXPIRES_MINUTES", value: "-5", wantInErr: "ADMIN_JWT_EXPIRES_MINUTES"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTT_ACTIVE_COUNTRY", "gt")
			t.Setenv(tc.key, tc.value)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() debía fallar con %s=%q, pero no devolvió error (cfg=%+v)", tc.key, tc.value, cfg)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("el error debía mencionar %q, se obtuvo: %v", tc.wantInErr, err)
			}
		})
	}
}
