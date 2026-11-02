package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"admin-backend/internal/users"
)

// Errores de validación del JWT de sesión del admin. Permiten al llamador
// distinguir las causas comunes de rechazo de un token (Req. 2.2).
var (
	// ErrEmptySecret se devuelve cuando se intenta emitir o validar un token
	// con un secreto de firma vacío. El secreto proviene de ADMIN_JWT_SECRET y
	// es obligatorio para HS256.
	ErrEmptySecret = errors.New("auth: el secreto de firma del JWT no puede estar vacío")

	// ErrInvalidToken se devuelve cuando el token no puede parsearse, su firma
	// no es válida, usa un método de firma inesperado o sus claims están mal
	// formados.
	ErrInvalidToken = errors.New("auth: el token de sesión es inválido")

	// ErrExpiredToken se devuelve cuando el token es sintácticamente válido y
	// está correctamente firmado, pero su fecha de expiración (claim exp) ya
	// pasó.
	ErrExpiredToken = errors.New("auth: el token de sesión ha expirado")
)

// sessionSigningMethod es el algoritmo de firma usado para el JWT de sesión del
// admin. El diseño especifica HS256 (clave simétrica compartida vía
// ADMIN_JWT_SECRET), a diferencia del JWT OTT que usa RS256.
var sessionSigningMethod = jwt.SigningMethodHS256

// SessionClaims representa los claims del JWT de sesión del admin emitido por el
// admin-backend tras un login exitoso. Embebe la identidad del administrador
// (id y username), sus permisos efectivos en forma compacta y, de forma
// opcional, su rol legacy, además de los claims registrados estándar (exp, iat)
// gestionados por jwt.RegisteredClaims.
//
// El claim `sub` (subject) transporta el id del usuario; `username` es un claim
// privado que la UI y los middlewares consumen; `perms` transporta los permisos
// efectivos compactos para la autorización por recurso/acción/país; `role` se
// conserva como claim legacy opcional durante la convivencia (Req. 1.6, 2.2,
// 7.2, 7.3, 7.4).
type SessionClaims struct {
	Username string             `json:"username"`
	Role     string             `json:"role,omitempty"`     // legacy, opcional durante la convivencia
	Roles    []string           `json:"roles,omitempty"`    // nombres de los roles RBAC asignados (para la UI)
	Perms    CompactPerms       `json:"perms,omitempty"`    // permisos efectivos compactos del catálogo estático (Req. 7.2, 7.3)
	Modperms CompactModulePerms `json:"modperms,omitempty"` // permisos de módulos dinámicos (module-builder, Req. 11.3, 11.4)
	jwt.RegisteredClaims
}

// UserID devuelve el identificador del usuario embebido en el claim `sub`.
func (c SessionClaims) UserID() string {
	return c.Subject
}

// EffectivePermissions decodifica el claim compacto `Perms` al conjunto de
// permisos efectivos del usuario, listo para consultas de autorización con
// HasPermission (Req. 7.2, 7.3). Tolera un claim vacío/nil devolviendo un
// conjunto vacío.
func (c SessionClaims) EffectivePermissions() EffectivePermissions {
	return DecodePermissions(c.Perms)
}

// EffectiveModulePermissions decodifica el claim `Modperms` al conjunto de
// permisos de módulos dinámicos del usuario, listo para consultas con
// HasModulePermission (Req. 11.3, 11.4, 11.5, 11.6). Tolera un claim vacío/nil
// devolviendo un conjunto vacío.
func (c SessionClaims) EffectiveModulePermissions() EffectiveModulePermissions {
	return DecodeModulePermissions(c.Modperms)
}

// IssueSessionToken emite un JWT de sesión del admin firmado con HS256 usando
// `secret` (ADMIN_JWT_SECRET). El token embebe la identidad del administrador
// (user.ID en `sub`, user.Username), sus permisos efectivos `perms` en forma
// compacta y su rol legacy, con una expiración derivada de `expiresMinutes`
// (ADMIN_JWT_EXPIRES_MINUTES) a partir del instante actual (Req. 1.6, 7.2,
// 7.3, 7.4).
//
// Los permisos se serializan con EncodePermissions para mantener el token
// pequeño y guardarlos en el claim `Perms`.
//
// Devuelve ErrEmptySecret si el secreto está vacío. `expiresMinutes` debe ser
// positivo; la validación de configuración garantiza esta condición al cargar.
func IssueSessionToken(user users.User, perms EffectivePermissions, roleNames []string, secret string, expiresMinutes int) (string, error) {
	// Variante sin permisos de módulo: conserva la firma y el comportamiento
	// previos para todos los llamadores existentes (login y pruebas).
	return IssueSessionTokenWithModules(user, perms, EffectiveModulePermissions{}, roleNames, secret, expiresMinutes)
}

// IssueSessionTokenWithModules emite el JWT de sesión incluyendo, además de los
// permisos estáticos `perms`, los permisos de módulos dinámicos `modPerms` en el
// claim `modperms` (Req. 11.3, 11.4). Es la variante que usa el login una vez
// que module-builder está activo; IssueSessionToken delega aquí con un conjunto
// de módulos vacío para mantener la retrocompatibilidad.
//
// El claim `modperms` se omite del token (omitempty) cuando el conjunto de
// módulos está vacío, de modo que los tokens que no usan módulos quedan
// idénticos a los emitidos antes de esta feature.
func IssueSessionTokenWithModules(user users.User, perms EffectivePermissions, modPerms EffectiveModulePermissions, roleNames []string, secret string, expiresMinutes int) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	if expiresMinutes <= 0 {
		return "", fmt.Errorf("auth: la expiración del token debe ser positiva, se obtuvo %d", expiresMinutes)
	}

	now := time.Now()
	compactMods := EncodeModulePermissions(modPerms)
	if len(compactMods) == 0 {
		// Asegura que omitempty surta efecto (mapa vacío vs nil).
		compactMods = nil
	}
	claims := SessionClaims{
		Username: user.Username,
		Role:     user.Role,
		Roles:    roleNames,
		Perms:    EncodePermissions(perms),
		Modperms: compactMods,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expiresMinutes) * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(sessionSigningMethod, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("auth: no se pudo firmar el token de sesión: %w", err)
	}

	return signed, nil
}

// ValidateSessionToken parsea y valida un JWT de sesión del admin: verifica que
// la firma HS256 corresponda a `secret` y que el token no haya expirado,
// devolviendo los claims (id, username, role) en caso de éxito (Req. 2.2).
//
// Devuelve ErrEmptySecret si el secreto está vacío, ErrExpiredToken si el token
// expiró, o ErrInvalidToken para cualquier otro fallo (firma inválida, método
// de firma inesperado, token malformado o claims incompletos).
func ValidateSessionToken(tokenString, secret string) (*SessionClaims, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}

	claims := &SessionClaims{}
	keyFunc := func(t *jwt.Token) (interface{}, error) {
		// Rechaza cualquier método de firma distinto del esperado para evitar
		// ataques de confusión de algoritmo (p. ej. "none" o RS256).
		if t.Method != sessionSigningMethod {
			return nil, fmt.Errorf("%w: método de firma inesperado %q", ErrInvalidToken, t.Header["alg"])
		}
		return []byte(secret), nil
	}

	parsed, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		keyFunc,
		jwt.WithValidMethods([]string{sessionSigningMethod.Alg()}),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !parsed.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
