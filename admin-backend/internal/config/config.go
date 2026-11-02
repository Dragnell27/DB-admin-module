// Package config carga la configuración del admin-backend desde variables de
// entorno. Ningún valor sensible (secretos, contraseñas, credenciales OTT) se
// embebe en el código: todo proviene del entorno (Req. 21.5, 23.3, 24.2).
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// SupportedCountries es el conjunto de países soportados por el BFF.
// OTT_ACTIVE_COUNTRY debe pertenecer a este conjunto (Req. 3.1).
var SupportedCountries = []string{"gt", "sv", "ni", "hn", "cr"}

// OTTCredentials contiene el par clientId/clientSecret de un país.
type OTTCredentials struct {
	ClientID     string
	ClientSecret string
}

// Config agrupa toda la configuración del admin-backend.
type Config struct {
	// Servidor HTTP
	Port              string
	CORSAllowedOrigin string

	// Base de datos (PostgreSQL)
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// JWT de sesión del admin
	AdminJWTSecret         string
	AdminJWTExpiresMinutes int

	// Clave de cifrado en reposo del constructor de módulos (module-builder).
	// Proviene de MODULE_SECRETS_KEY (base64 de 32 bytes = AES-256) y se expone
	// ya decodificada para que internal/secrets la use directamente. Es
	// OPCIONAL en la carga: si está ausente, ModuleSecretsKey queda nil y la
	// feature module-builder no puede cifrar/descifrar (quien la necesite debe
	// comprobarlo). Se decidió opcional para no romper arranques ni tests
	// existentes que no definen la variable; cuando está presente, se valida
	// que sea base64 válido de exactamente 32 bytes (ver validate).
	ModuleSecretsKey []byte

	// Hashing de contraseñas
	BcryptCost int

	// Caché de lecturas OTT: minutos de vigencia (TTL) de cada entrada del
	// caché en memoria del BFF. Configurable con OTT_CACHE_TTL_MINUTES; un valor
	// no positivo o inválido cae al valor por defecto (10).
	OTTCacheTTLMinutes int

	// Seed del administrador inicial
	SeedAdminUsername string
	SeedAdminPassword string
	SeedAdminRole     string

	// Integración con el backend OTT.
	//
	// La autenticación y los recursos viven en hosts distintos en el ambiente
	// de CENAM, por lo que se configuran por separado:
	//   - OTTAuthBaseURL: base del endpoint de autenticación (POST /authenticate).
	//     p. ej. https://api-qa-paymentgp.portalpagoscenam.com/api/v1
	//   - OTTBaseURL: base de los recursos (customers/plans/subscriptions/cards).
	//     p. ej. https://api-ott-qa.portalpagoscenam.com
	OTTAuthBaseURL   string
	OTTBaseURL       string
	OTTActiveCountry string

	// Credenciales OTT por país (clave: código de país en minúsculas)
	OTTCredentialsByCountry map[string]OTTCredentials
}

// Load lee la configuración desde variables de entorno, aplica valores por
// defecto no sensibles y valida las invariantes de arranque. Devuelve un error
// si OTT_ACTIVE_COUNTRY no pertenece al conjunto {gt,sv,ni,hn,cr} o si alguna
// variable con formato inválido impide un arranque seguro.
func Load() (*Config, error) {
	// En desarrollo local el binario puede leer un archivo .env del
	// admin-backend para no tener que exportar las variables a mano. Las
	// variables ya presentes en el entorno del proceso tienen prioridad sobre
	// el archivo. En contenedores no suele existir el .env y la configuración
	// llega por el entorno (docker-compose), por lo que esto es un no-op.
	loadDotEnvIfPresent()

	cfg := &Config{
		Port:              getEnvDefault("PORT", "8081"),
		CORSAllowedOrigin: getEnvDefault("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),

		DBHost:     getEnvDefault("DB_HOST", "localhost"),
		DBPort:     getEnvDefault("DB_PORT", "5432"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:  getEnvDefault("DB_SSLMODE", "disable"),

		AdminJWTSecret: os.Getenv("ADMIN_JWT_SECRET"),

		SeedAdminUsername: getEnvDefault("SEED_ADMIN_USERNAME", "admin"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),
		SeedAdminRole:     getEnvDefault("SEED_ADMIN_ROLE", "admin"),

		OTTBaseURL:       getEnvDefault("OTT_BASE_URL", "http://localhost:8080/api/v1"),
		OTTActiveCountry: strings.ToLower(strings.TrimSpace(os.Getenv("OTT_ACTIVE_COUNTRY"))),
	}

	// La base de autenticación OTT es independiente de la de recursos. Si no se
	// configura OTT_AUTH_BASE_URL, se usa OTT_BASE_URL como respaldo (comportamiento
	// previo: auth y recursos en el mismo host).
	cfg.OTTAuthBaseURL = getEnvDefault("OTT_AUTH_BASE_URL", cfg.OTTBaseURL)

	// Clave de cifrado de module-builder (opcional). Si está presente, debe ser
	// base64 válido de exactamente 32 bytes; si está ausente, se deja nil.
	moduleKey, err := decodeModuleSecretsKey(os.Getenv("MODULE_SECRETS_KEY"))
	if err != nil {
		return nil, err
	}
	cfg.ModuleSecretsKey = moduleKey

	// Enteros con valores por defecto no sensibles.
	expiresMinutes, err := getEnvInt("ADMIN_JWT_EXPIRES_MINUTES", 480)
	if err != nil {
		return nil, err
	}
	cfg.AdminJWTExpiresMinutes = expiresMinutes

	bcryptCost, err := getEnvInt("BCRYPT_COST", 12)
	if err != nil {
		return nil, err
	}
	cfg.BcryptCost = bcryptCost

	// TTL del caché de lecturas OTT (minutos). A diferencia de otros enteros,
	// un valor inválido o no positivo NO debe impedir el arranque: se cae al
	// valor por defecto (10). Por eso no se usa getEnvInt (que propaga error).
	cfg.OTTCacheTTLMinutes = getEnvIntDefault("OTT_CACHE_TTL_MINUTES", 10)

	// Credenciales OTT por país leídas desde entorno (nunca embebidas).
	cfg.OTTCredentialsByCountry = make(map[string]OTTCredentials, len(SupportedCountries))
	for _, country := range SupportedCountries {
		prefix := "OTT_" + strings.ToUpper(country)
		cfg.OTTCredentialsByCountry[country] = OTTCredentials{
			ClientID:     os.Getenv(prefix + "_CLIENT_ID"),
			ClientSecret: os.Getenv(prefix + "_CLIENT_SECRET"),
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate comprueba las invariantes de arranque.
func (c *Config) validate() error {
	if c.OTTActiveCountry == "" {
		return fmt.Errorf("OTT_ACTIVE_COUNTRY es obligatorio y debe ser uno de %v", SupportedCountries)
	}
	if !isSupportedCountry(c.OTTActiveCountry) {
		return fmt.Errorf("OTT_ACTIVE_COUNTRY %q no es soportado; debe ser uno de %v", c.OTTActiveCountry, SupportedCountries)
	}
	if c.BcryptCost <= 0 {
		return fmt.Errorf("BCRYPT_COST debe ser un entero positivo, se obtuvo %d", c.BcryptCost)
	}
	if c.AdminJWTExpiresMinutes <= 0 {
		return fmt.Errorf("ADMIN_JWT_EXPIRES_MINUTES debe ser un entero positivo, se obtuvo %d", c.AdminJWTExpiresMinutes)
	}
	return nil
}

// ConfiguredCountries devuelve, en el orden de SupportedCountries, el subconjunto
// de países que tienen credenciales OTT completas (ClientID y ClientSecret no
// vacíos). Es la lista de países que el BFF puede consultar realmente y la que
// el frontend usa para poblar el selector de país. El país activo por defecto
// (OTTActiveCountry) se incluye si tiene credenciales como cualquier otro.
func (c *Config) ConfiguredCountries() []string {
	out := make([]string, 0, len(SupportedCountries))
	for _, country := range SupportedCountries {
		creds, ok := c.OTTCredentialsByCountry[country]
		if !ok {
			continue
		}
		if strings.TrimSpace(creds.ClientID) == "" || strings.TrimSpace(creds.ClientSecret) == "" {
			continue
		}
		out = append(out, country)
	}
	return out
}

// isSupportedCountry indica si el país pertenece al conjunto soportado.
func isSupportedCountry(country string) bool {
	for _, c := range SupportedCountries {
		if c == country {
			return true
		}
	}
	return false
}

// getEnvDefault devuelve el valor de la variable o el valor por defecto si está
// vacía o ausente.
func getEnvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// decodeModuleSecretsKey interpreta el valor crudo de MODULE_SECRETS_KEY. Si
// está vacío o ausente devuelve (nil, nil): la clave es opcional y su ausencia
// no impide el arranque (la feature module-builder la exigirá cuando se use).
// Si está presente, debe ser base64 (estándar) de EXACTAMENTE 32 bytes
// (AES-256); en caso contrario devuelve un error de arranque claro. No incluye
// el valor de la clave en el mensaje de error.
func decodeModuleSecretsKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("MODULE_SECRETS_KEY debe ser base64 válido: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("MODULE_SECRETS_KEY debe decodificar a 32 bytes (AES-256), se obtuvieron %d", len(key))
	}
	return key, nil
}

// getEnvInt lee una variable entera, aplicando un valor por defecto si está
// ausente y devolviendo error si el valor presente no es un entero válido.
func getEnvInt(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s debe ser un entero válido: %w", key, err)
	}
	return v, nil
}

// getEnvIntDefault lee una variable entera de forma TOLERANTE: si está ausente,
// vacía, no numérica o no positiva, devuelve el valor por defecto sin propagar
// error. Útil para parámetros no críticos para el arranque (p. ej. el TTL del
// caché), donde un valor inválido no debe impedir levantar el servicio.
func getEnvIntDefault(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// OTTCacheTTL devuelve la vigencia (TTL) de las entradas del caché de lecturas
// OTT como time.Duration, derivada de OTTCacheTTLMinutes. Un valor no positivo
// (que no debería ocurrir tras la carga tolerante) cae a 10 minutos.
func (c *Config) OTTCacheTTL() time.Duration {
	minutes := c.OTTCacheTTLMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes) * time.Minute
}
