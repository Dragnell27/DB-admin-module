package api

import "testing"

// TestRedact_StripsNestedConnectionSecrets verifica que redact() elimina los
// contenedores `secrets` (con dsn/variableValues anidados) y `fixedValues` del
// cuerpo auditado, cerrando también la fuga preexistente de secrets.dsn
// (Req. 4.4).
func TestRedact_StripsNestedConnectionSecrets(t *testing.T) {
	body := map[string]interface{}{
		"slug": "mi-modulo",
		"name": "Mi Módulo",
		"secrets": map[string]interface{}{
			"dsn":            "postgres://u:p@h/db",
			"variableValues": map[string]interface{}{"dbname": "recaudo_gt"},
		},
		"fixedValues": map[string]interface{}{
			"password": "s3cr3t",
			"host":     "10.0.0.5",
		},
	}

	out := redact(body)

	if _, ok := out["secrets"]; ok {
		t.Error("la clave contenedora `secrets` debería eliminarse")
	}
	if _, ok := out["fixedValues"]; ok {
		t.Error("la clave contenedora `fixedValues` debería eliminarse")
	}
	// Las claves NO sensibles se conservan.
	if out["slug"] != "mi-modulo" || out["name"] != "Mi Módulo" {
		t.Errorf("las claves no sensibles deberían conservarse: %+v", out)
	}
}

// TestRedact_StripsTopLevelSensitive verifica que las claves sensibles de primer
// nivel ya cubiertas siguen eliminándose (no regresión).
func TestRedact_StripsTopLevelSensitive(t *testing.T) {
	body := map[string]interface{}{
		"username": "admin",
		"password": "1234",
		"dsn":      "postgres://u:p@h/db",
		"apikey":   "abc",
	}
	out := redact(body)
	for _, k := range []string{"password", "dsn", "apikey"} {
		if _, ok := out[k]; ok {
			t.Errorf("la clave sensible %q debería eliminarse", k)
		}
	}
	if out["username"] != "admin" {
		t.Error("username debería conservarse")
	}
}
