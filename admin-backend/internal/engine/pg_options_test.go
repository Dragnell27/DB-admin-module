package engine

import (
	"errors"
	"reflect"
	"testing"

	"admin-backend/internal/modules"
)

// optionsWhitelist común de las pruebas del constructor de la consulta de
// opciones de un origen dinámico.
var optionsWhitelist = map[string][]string{
	"paises": {"id", "nombre", "activo", "orden"},
}

// ptrOrder es un helper para declarar un *OrderSpec en los casos de prueba.
func ptrOrder(col, dir string) *modules.OrderSpec {
	return &modules.OrderSpec{Column: col, Direction: dir}
}

// TestBuildOptionsSQL verifica el SQL EXACTO que genera buildOptionsSQL (solo
// id+label; con WHERE; con ORDER BY asc/desc; con LIMIT por defecto y acotado),
// la numeración $N del WHERE y del LIMIT, el aborto con ErrWhitelist SIN SQL por
// identificadores fuera de lista blanca, y ErrValidation por campos faltantes.
func TestBuildOptionsSQL(t *testing.T) {
	cases := []struct {
		name     string
		src      modules.DynamicSource
		wantSQL  string
		wantArgs []any
		wantErr  error
	}{
		{
			name:     "solo id+label ⇒ SELECT con LIMIT por defecto",
			src:      modules.DynamicSource{Table: "paises", IDColumn: "id", LabelColumn: "nombre"},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" LIMIT $1`,
			wantArgs: []any{DefaultOptionsLimit},
		},
		{
			name: "con WHERE ⇒ filtros parametrizados y LIMIT numerado después",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Filters: []modules.ListCondition{{Field: "activo", Values: []string{"1"}}},
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" WHERE lower(btrim("activo"::text)) IN ($1) LIMIT $2`,
			wantArgs: []any{"1", DefaultOptionsLimit},
		},
		{
			name: "con ORDER BY asc",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Order: ptrOrder("nombre", "asc"),
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" ORDER BY "nombre" asc LIMIT $1`,
			wantArgs: []any{DefaultOptionsLimit},
		},
		{
			name: "con ORDER BY desc",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Order: ptrOrder("orden", "desc"),
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" ORDER BY "orden" desc LIMIT $1`,
			wantArgs: []any{DefaultOptionsLimit},
		},
		{
			name: "WHERE + ORDER BY ⇒ orden de cláusulas y numeración",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Filters: []modules.ListCondition{{Field: "activo", Values: []string{"1", "2"}}},
				Order:   ptrOrder("nombre", "asc"),
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" WHERE lower(btrim("activo"::text)) IN ($1, $2) ORDER BY "nombre" asc LIMIT $3`,
			wantArgs: []any{"1", "2", DefaultOptionsLimit},
		},
		{
			name: "LIMIT acotado (válido) se respeta",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre", Limit: 10,
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" LIMIT $1`,
			wantArgs: []any{10},
		},
		{
			name: "LIMIT fuera de rango cae al tope por defecto",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre", Limit: 100000,
			},
			wantSQL:  `SELECT "id", "nombre" FROM "paises" LIMIT $1`,
			wantArgs: []any{DefaultOptionsLimit},
		},
		{
			name:    "tabla fuera de lista blanca ⇒ ErrWhitelist sin SQL",
			src:     modules.DynamicSource{Table: "secreta", IDColumn: "id", LabelColumn: "nombre"},
			wantErr: ErrWhitelist,
		},
		{
			name:    "columna id fuera de lista blanca ⇒ ErrWhitelist sin SQL",
			src:     modules.DynamicSource{Table: "paises", IDColumn: "oculto", LabelColumn: "nombre"},
			wantErr: ErrWhitelist,
		},
		{
			name:    "columna etiqueta fuera de lista blanca ⇒ ErrWhitelist sin SQL",
			src:     modules.DynamicSource{Table: "paises", IDColumn: "id", LabelColumn: "oculto"},
			wantErr: ErrWhitelist,
		},
		{
			name: "columna de filtro fuera de lista blanca ⇒ ErrWhitelist sin SQL",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Filters: []modules.ListCondition{{Field: "secreto", Values: []string{"x"}}},
			},
			wantErr: ErrWhitelist,
		},
		{
			name: "columna de orden fuera de lista blanca ⇒ ErrWhitelist sin SQL",
			src: modules.DynamicSource{
				Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				Order: ptrOrder("secreto", "asc"),
			},
			wantErr: ErrWhitelist,
		},
		{
			name:    "falta tabla ⇒ ErrValidation sin SQL",
			src:     modules.DynamicSource{IDColumn: "id", LabelColumn: "nombre"},
			wantErr: ErrValidation,
		},
		{
			name:    "falta columna id ⇒ ErrValidation sin SQL",
			src:     modules.DynamicSource{Table: "paises", LabelColumn: "nombre"},
			wantErr: ErrValidation,
		},
		{
			name:    "falta columna etiqueta ⇒ ErrValidation sin SQL",
			src:     modules.DynamicSource{Table: "paises", IDColumn: "id"},
			wantErr: ErrValidation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := buildOptionsSQL(optionsWhitelist, tc.src)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("se esperaba error, got nil (sql=%q)", sql)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("el error debe envolver %v, got %v", tc.wantErr, err)
				}
				if sql != "" || args != nil {
					t.Fatalf("en error no debe producirse SQL: sql=%q args=%v", sql, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if sql != tc.wantSQL {
				t.Fatalf("sql\n  got:  %s\n  want: %s", sql, tc.wantSQL)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("args\n  got:  %v\n  want: %v", args, tc.wantArgs)
			}
		})
	}
}

// TestNormalizeDirection verifica que entradas basura caen a "asc" y que solo
// "desc" (sin distinguir mayúsculas) produce "desc".
func TestNormalizeDirection(t *testing.T) {
	cases := map[string]string{
		"desc":    "desc",
		"DESC":    "desc",
		" Desc ":  "desc",
		"asc":     "asc",
		"ASC":     "asc",
		"":        "asc",
		"basura":  "asc",
		"ascendg": "asc",
	}
	for in, want := range cases {
		if got := normalizeDirection(in); got != want {
			t.Fatalf("normalizeDirection(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEffectiveLimit verifica el tope por defecto para 0, negativos y >500, y el
// respeto de un valor válido.
func TestEffectiveLimit(t *testing.T) {
	cases := map[int]int{
		0:                       DefaultOptionsLimit,
		-5:                      DefaultOptionsLimit,
		DefaultOptionsLimit + 1: DefaultOptionsLimit,
		100000:                  DefaultOptionsLimit,
		1:                       1,
		250:                     250,
		DefaultOptionsLimit:     DefaultOptionsLimit,
	}
	for in, want := range cases {
		if got := effectiveLimit(in); got != want {
			t.Fatalf("effectiveLimit(%d) = %d, want %d", in, got, want)
		}
	}
}
