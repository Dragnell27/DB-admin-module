package connections

import (
	"net/url"
	"strings"
	"testing"
)

// parse ayuda a comparar DSNs por componentes en los tests (evita falsos
// negativos por orden de query params).
func parseDSN(t *testing.T, dsn string) *url.URL {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("DSN inválido %q: %v", dsn, err)
	}
	return u
}

func TestAssembleDSN_FixedAndVariable(t *testing.T) {
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
		{Key: "port", Kind: FieldFixed, Role: RolePort},
		{Key: "user", Kind: FieldFixed, Role: RoleUser},
		{Key: "password", Kind: FieldFixed, Role: RolePassword},
		{Key: "dbname", Kind: FieldVariable, Role: RoleDBName},
	}
	fixed := map[string]string{"host": "10.0.0.5", "port": "5432", "user": "svc", "password": "s3cr3t"}
	variable := map[string]string{"dbname": "recaudo_gt"}

	dsn, err := AssembleDSN(fields, fixed, variable)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	u := parseDSN(t, dsn)
	if u.Scheme != "postgres" {
		t.Errorf("scheme = %q, se esperaba postgres", u.Scheme)
	}
	if u.Hostname() != "10.0.0.5" || u.Port() != "5432" {
		t.Errorf("host:port = %q", u.Host)
	}
	if u.User.Username() != "svc" {
		t.Errorf("user = %q", u.User.Username())
	}
	pw, _ := u.User.Password()
	if pw != "s3cr3t" {
		t.Errorf("password = %q", pw)
	}
	if strings.TrimPrefix(u.Path, "/") != "recaudo_gt" {
		t.Errorf("dbname = %q", u.Path)
	}
}

func TestAssembleDSN_PasswordEscaping(t *testing.T) {
	fields := []TemplateField{
		{Key: "user", Kind: FieldFixed, Role: RoleUser},
		{Key: "password", Kind: FieldFixed, Role: RolePassword},
		{Key: "dbname", Kind: FieldFixed, Role: RoleDBName},
	}
	fixed := map[string]string{"user": "us er", "password": "p@ss:w/rd?", "dbname": "db"}
	dsn, err := AssembleDSN(fields, fixed, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	u := parseDSN(t, dsn)
	pw, _ := u.User.Password()
	if pw != "p@ss:w/rd?" {
		t.Errorf("password mal escapado: %q", pw)
	}
	if u.User.Username() != "us er" {
		t.Errorf("user mal escapado: %q", u.User.Username())
	}
}

func TestAssembleDSN_NonNumericPort(t *testing.T) {
	fields := []TemplateField{
		{Key: "port", Kind: FieldFixed, Role: RolePort},
		{Key: "dbname", Kind: FieldFixed, Role: RoleDBName},
	}
	_, err := AssembleDSN(fields, map[string]string{"port": "abc", "dbname": "db"}, nil)
	if err == nil {
		t.Fatal("se esperaba error por puerto no numérico")
	}
}

func TestAssembleDSN_HostAbsentIsValid(t *testing.T) {
	fields := []TemplateField{
		{Key: "dbname", Kind: FieldFixed, Role: RoleDBName},
	}
	dsn, err := AssembleDSN(fields, map[string]string{"dbname": "db"}, nil)
	if err != nil {
		t.Fatalf("host ausente debe ser válido: %v", err)
	}
	u := parseDSN(t, dsn)
	if u.Host != "" {
		t.Errorf("host debería omitirse, got %q", u.Host)
	}
	if strings.TrimPrefix(u.Path, "/") != "db" {
		t.Errorf("dbname = %q", u.Path)
	}
}

func TestAssembleDSN_DBNameRequired(t *testing.T) {
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
	}
	_, err := AssembleDSN(fields, map[string]string{"host": "h"}, nil)
	if err == nil {
		t.Fatal("se esperaba error por dbname ausente")
	}
}

func TestAssembleDSN_SSLModeAndOptions(t *testing.T) {
	fields := []TemplateField{
		{Key: "dbname", Kind: FieldFixed, Role: RoleDBName},
		{Key: "sslmode", Kind: FieldFixed, Role: RoleSSLMode},
		{Key: "connect_timeout", Kind: FieldFixed, Role: RoleOption},
		{Key: "application_name", Kind: FieldFixed, Role: ""}, // role vacío = option
	}
	fixed := map[string]string{
		"dbname":           "db",
		"sslmode":          "require",
		"connect_timeout":  "10",
		"application_name": "admin",
	}
	dsn, err := AssembleDSN(fields, fixed, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	u := parseDSN(t, dsn)
	q := u.Query()
	if q.Get("sslmode") != "require" {
		t.Errorf("sslmode = %q", q.Get("sslmode"))
	}
	if q.Get("connect_timeout") != "10" {
		t.Errorf("connect_timeout reemitido mal: %q", q.Get("connect_timeout"))
	}
	if q.Get("application_name") != "admin" {
		t.Errorf("application_name reemitido mal: %q", q.Get("application_name"))
	}
}

func TestAssembleDSN_AllFixedEmptyVariables(t *testing.T) {
	// Caso del mapeo 1:1 de la migración: todos FIJOS, variables {}.
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
		{Key: "dbname", Kind: FieldFixed, Role: RoleDBName},
	}
	fixed := map[string]string{"host": "h", "dbname": "db"}
	dsn, err := AssembleDSN(fields, fixed, map[string]string{})
	if err != nil {
		t.Fatalf("variables {} debe ser válido: %v", err)
	}
	u := parseDSN(t, dsn)
	if u.Hostname() != "h" || strings.TrimPrefix(u.Path, "/") != "db" {
		t.Errorf("DSN inesperado: %q", dsn)
	}
}

func TestParseLegacyDSN_URLForm(t *testing.T) {
	comp, err := parseLegacyDSN("postgres://svc:s3cr3t@10.0.0.5:5432/recaudo_gt?sslmode=require&connect_timeout=10")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if comp.Host != "10.0.0.5" || comp.Port != "5432" {
		t.Errorf("host/port = %q/%q", comp.Host, comp.Port)
	}
	if comp.User != "svc" || comp.Password != "s3cr3t" {
		t.Errorf("user/password = %q/%q", comp.User, comp.Password)
	}
	if comp.DBName != "recaudo_gt" {
		t.Errorf("dbname = %q", comp.DBName)
	}
	if comp.SSLMode != "require" {
		t.Errorf("sslmode = %q", comp.SSLMode)
	}
	if comp.Options["connect_timeout"] != "10" {
		t.Errorf("connect_timeout = %q", comp.Options["connect_timeout"])
	}
}

func TestParseLegacyDSN_KeyValueForm(t *testing.T) {
	comp, err := parseLegacyDSN("host=10.0.0.5 port=5432 user=svc password=s3cr3t dbname=db application_name=admin")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if comp.Host != "10.0.0.5" || comp.DBName != "db" {
		t.Errorf("componentes = %+v", comp)
	}
	if comp.Options["application_name"] != "admin" {
		t.Errorf("application_name = %q", comp.Options["application_name"])
	}
}

func TestFieldsFromComponents_RoundTrip(t *testing.T) {
	// Un DSN legado con parámetros extra debe round-tripear idéntico por
	// componentes al re-ensamblar.
	legacy := "postgres://svc:s3cr3t@10.0.0.5:5432/db?sslmode=require&connect_timeout=10&application_name=admin"
	comp, err := parseLegacyDSN(legacy)
	if err != nil {
		t.Fatalf("parse legacy: %v", err)
	}
	fields, values := fieldsFromComponents(comp)
	dsn, err := AssembleDSN(fields, values, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	reparsed, err := parseLegacyDSN(dsn)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if !dsnEquivalent(comp, reparsed) {
		t.Errorf("round-trip no equivalente:\n legacy=%+v\n reparsed=%+v", comp, reparsed)
	}
}
