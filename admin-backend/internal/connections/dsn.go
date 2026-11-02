package connections

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// dsnComponents son los componentes normalizables de un DSN PostgreSQL. `options`
// conserva los parámetros libpq extra con su NOMBRE ORIGINAL como clave (sin
// renombrar). Se usa tanto para ensamblar como para la verificación de
// equivalencia de la migración.
type dsnComponents struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	Options  map[string]string // parámetros libpq extra, nombre original -> valor
}

// standardLibpqKeys son las claves libpq con mapeo directo a un componente del
// DSN; el resto se preserva en Options con su nombre original.
var standardLibpqKeys = map[string]struct{}{
	"host":     {},
	"port":     {},
	"user":     {},
	"password": {},
	"dbname":   {},
	"sslmode":  {},
}

// AssembleDSN construye un DSN PostgreSQL `postgres://user:password@host:port/dbname?params`
// combinando los campos FIJOS y VARIABLE según el `role` de cada TemplateField.
// Es una función PURA (no inyecta Sealer). Reglas (D-1):
//   - dbname es el ÚNICO componente estrictamente requerido (si falta → error).
//   - host es OPCIONAL: si no hay, se omite de la URL (el driver usa su default).
//   - port se omite si no está presente; si está presente debe ser numérico.
//   - sslmode y el resto de parámetros (role option o vacío) van a la query, con
//     su nombre original (la key del campo para option/vacío).
//
// El valor de cada clave se toma de `variable[key]` si el campo es VARIABLE o de
// `fixed[key]` si es FIJO. Un valor ausente/vacío simplemente no se emite (salvo
// dbname, que es obligatorio).
func AssembleDSN(fields []TemplateField, fixed, variable map[string]string) (string, error) {
	comp := dsnComponents{Options: map[string]string{}}

	for _, f := range fields {
		val := valueForField(f, fixed, variable)
		role := f.Role
		if role == "" {
			role = RoleOption
		}
		switch role {
		case RoleHost:
			comp.Host = val
		case RolePort:
			comp.Port = val
		case RoleUser:
			comp.User = val
		case RolePassword:
			comp.Password = val
		case RoleDBName:
			comp.DBName = val
		case RoleSSLMode:
			comp.SSLMode = val
		case RoleOption:
			// El parámetro libpq usa el nombre de la clave del campo.
			if val != "" {
				comp.Options[f.Key] = val
			}
		default:
			return "", fmt.Errorf("%w: role %q no reconocido en el campo %q", ErrValidation, f.Role, f.Key)
		}
	}

	return assembleFromComponents(comp)
}

// valueForField resuelve el valor de un campo: de `variable` si es VARIABLE, de
// `fixed` si es FIJO.
func valueForField(f TemplateField, fixed, variable map[string]string) string {
	if f.Kind == FieldVariable {
		return strings.TrimSpace(variable[f.Key])
	}
	return strings.TrimSpace(fixed[f.Key])
}

// assembleFromComponents construye la URL postgres:// desde componentes ya
// resueltos. Valida que dbname esté presente y que el port, si está, sea
// numérico. host ausente se omite (default del driver).
func assembleFromComponents(comp dsnComponents) (string, error) {
	if strings.TrimSpace(comp.DBName) == "" {
		return "", fmt.Errorf("%w: falta el componente dbname para formar el DSN", ErrValidation)
	}
	if comp.Port != "" {
		if _, err := strconv.Atoi(comp.Port); err != nil {
			return "", fmt.Errorf("%w: el puerto %q no es numérico", ErrValidation, comp.Port)
		}
	}

	u := &url.URL{Scheme: "postgres"}

	// userinfo: usuario y, si hay, password (url.UserPassword escapa).
	if comp.User != "" {
		if comp.Password != "" {
			u.User = url.UserPassword(comp.User, comp.Password)
		} else {
			u.User = url.User(comp.User)
		}
	}

	// host[:port]. host opcional (se omite si vacío).
	if comp.Host != "" {
		if comp.Port != "" {
			u.Host = net_JoinHostPort(comp.Host, comp.Port)
		} else {
			u.Host = comp.Host
		}
	} else if comp.Port != "" {
		// Sin host explícito pero con puerto: host vacío con puerto.
		u.Host = ":" + comp.Port
	}

	u.Path = "/" + comp.DBName

	// query: sslmode + options (nombre original). Orden estable para
	// determinismo (clave ordenada).
	q := url.Values{}
	if comp.SSLMode != "" {
		q.Set("sslmode", comp.SSLMode)
	}
	keys := make([]string, 0, len(comp.Options))
	for k := range comp.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Set(k, comp.Options[k])
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

// net_JoinHostPort une host y puerto respetando IPv6 (equivalente a
// net.JoinHostPort, sin importar net directamente en este punto).
func net_JoinHostPort(host, port string) string {
	if strings.ContainsAny(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

// parseLegacyDSN descompone un DSN legado (acepta `postgres://...` y formato
// libpq `clave=valor`) en componentes, preservando TODOS los parámetros no
// estándar en Options con su nombre libpq ORIGINAL (sin renombrar). Es una
// función pura usada por la migración.
func parseLegacyDSN(dsn string) (dsnComponents, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsnComponents{}, fmt.Errorf("%w: DSN vacío", ErrValidation)
	}
	comp := dsnComponents{Options: map[string]string{}}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsnComponents{}, fmt.Errorf("%w: DSN URL inválido: %v", ErrValidation, err)
		}
		comp.Host = u.Hostname()
		comp.Port = u.Port()
		if u.User != nil {
			comp.User = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				comp.Password = pw
			}
		}
		comp.DBName = strings.TrimPrefix(u.Path, "/")
		for k, vals := range u.Query() {
			if len(vals) == 0 {
				continue
			}
			v := vals[0]
			if k == "sslmode" {
				comp.SSLMode = v
			} else if _, std := standardLibpqKeys[k]; !std && v != "" {
				comp.Options[k] = v
			}
		}
		return comp, nil
	}

	// Formato libpq `clave=valor` separado por espacios.
	for _, tok := range strings.Fields(dsn) {
		kv := strings.SplitN(tok, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		switch k {
		case "host":
			comp.Host = v
		case "port":
			comp.Port = v
		case "user":
			comp.User = v
		case "password":
			comp.Password = v
		case "dbname":
			comp.DBName = v
		case "sslmode":
			comp.SSLMode = v
		default:
			if v != "" {
				comp.Options[k] = v
			}
		}
	}
	return comp, nil
}

// dsnEquivalent compara dos conjuntos de componentes por valor normalizado (no
// por string crudo), para la verificación de la migración:
//   - host: minúsculas; ausencia en ambos lados se considera igual.
//   - port: default 5432 cuando está ausente.
//   - user/password/dbname: igualdad exacta.
//   - sslmode: ausencia equivale a "prefer".
//   - options: conjunto EXACTO (mismas claves, mismos valores, nombre original).
//
// Es una función pura; nunca loguea valores.
func dsnEquivalent(a, b dsnComponents) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	normPort := func(p string) string {
		if strings.TrimSpace(p) == "" {
			return "5432"
		}
		return strings.TrimSpace(p)
	}
	normSSL := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "prefer"
		}
		return strings.TrimSpace(s)
	}
	if norm(a.Host) != norm(b.Host) {
		return false
	}
	if normPort(a.Port) != normPort(b.Port) {
		return false
	}
	if a.User != b.User || a.Password != b.Password || a.DBName != b.DBName {
		return false
	}
	if normSSL(a.SSLMode) != normSSL(b.SSLMode) {
		return false
	}
	if len(a.Options) != len(b.Options) {
		return false
	}
	for k, v := range a.Options {
		if b.Options[k] != v {
			return false
		}
	}
	return true
}

// fieldsFromComponents deriva la estructura de campos (todos FIJOS) a partir de
// componentes parseados de un DSN legado, para la migración 1:1. Solo incluye
// los componentes presentes. El orden es estable.
func fieldsFromComponents(comp dsnComponents) ([]TemplateField, FixedValues) {
	fields := make([]TemplateField, 0, 6+len(comp.Options))
	values := FixedValues{}

	addFixed := func(key, role, val string) {
		fields = append(fields, TemplateField{Key: key, Kind: FieldFixed, Role: role})
		values[key] = val
	}

	if comp.Host != "" {
		addFixed("host", RoleHost, comp.Host)
	}
	if comp.Port != "" {
		addFixed("port", RolePort, comp.Port)
	}
	if comp.User != "" {
		addFixed("user", RoleUser, comp.User)
	}
	if comp.Password != "" {
		addFixed("password", RolePassword, comp.Password)
	}
	if comp.DBName != "" {
		addFixed("dbname", RoleDBName, comp.DBName)
	}
	if comp.SSLMode != "" {
		addFixed("sslmode", RoleSSLMode, comp.SSLMode)
	}
	optKeys := make([]string, 0, len(comp.Options))
	for k := range comp.Options {
		optKeys = append(optKeys, k)
	}
	sort.Strings(optKeys)
	for _, k := range optKeys {
		addFixed(k, RoleOption, comp.Options[k])
	}
	return fields, values
}
