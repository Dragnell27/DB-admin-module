package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// dotEnvFileName es el nombre del archivo de variables de entorno para
// desarrollo local.
const dotEnvFileName = ".env"

// loadDotEnvIfPresent carga, si existe, un archivo `.env` en variables de
// entorno del proceso para facilitar el desarrollo local (ejecutar
// `go run ./cmd/api` sin exportar cada variable a mano).
//
// Reglas de resolución y precedencia:
//   - Si ADMIN_BACKEND_ENV_FILE está definido, se usa esa ruta.
//   - Si no, se busca un `.env` en el directorio de trabajo actual y, de no
//     encontrarse, subiendo hasta 3 niveles (para funcionar tanto si se corre
//     desde `admin-backend/` como desde la raíz del monorepo).
//   - Una variable que YA exista en el entorno del proceso NO se sobrescribe:
//     el entorno real (p. ej. el que inyecta docker-compose o la terminal)
//     siempre tiene prioridad sobre el archivo.
//
// Es intencionalmente tolerante: cualquier error de lectura o una línea mal
// formada se ignora silenciosamente (el .env es una comodidad de desarrollo,
// no una fuente de configuración obligatoria).
func loadDotEnvIfPresent() {
	path := resolveDotEnvPath()
	if path == "" {
		return
	}
	applyDotEnvFile(path)
}

// resolveDotEnvPath determina qué archivo .env cargar, o "" si no hay ninguno.
func resolveDotEnvPath() string {
	if explicit := strings.TrimSpace(os.Getenv("ADMIN_BACKEND_ENV_FILE")); explicit != "" {
		if fileExists(explicit) {
			return explicit
		}
		return ""
	}

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Busca en el cwd y hasta 3 niveles hacia arriba.
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, dotEnvFileName)
		if fileExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// applyDotEnvFile parsea el archivo e inyecta las variables ausentes.
func applyDotEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := parseDotEnvLine(scanner.Text())
		if !ok {
			continue
		}
		// No sobrescribir variables ya presentes en el entorno.
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

// parseDotEnvLine interpreta una línea `CLAVE=valor` del archivo .env.
// Devuelve ok=false para líneas en blanco, comentarios o sin '='.
// Soporta el prefijo opcional "export ", comillas simples/dobles alrededor del
// valor y comentarios al final de líneas con valor sin comillas.
func parseDotEnvLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:eq])
	if key == "" {
		return "", "", false
	}

	value = strings.TrimSpace(line[eq+1:])
	value = unquoteDotEnvValue(value)
	return key, value, true
}

// unquoteDotEnvValue quita comillas envolventes de un valor y, si el valor no
// está entrecomillado, recorta un comentario al final (" # ...").
func unquoteDotEnvValue(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	// Valor sin comillas: un '#' precedido de espacio inicia un comentario.
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

// fileExists indica si la ruta existe y es un archivo regular.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
