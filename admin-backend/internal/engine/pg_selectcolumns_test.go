package engine

import (
	"errors"
	"reflect"
	"testing"

	"admin-backend/internal/modules"
)

// Verifica que selectColumns calcula la UNIÓN de columnas de visualización
// (lista blanca) y columnas de condición, en orden estable y sin duplicados, y
// que valida cada columna de condición contra la lista blanca de la tabla.
func TestSelectColumns(t *testing.T) {
	whitelist := map[string][]string{
		"planes": {"id", "nombre", "eliminado"},
	}

	cases := []struct {
		name       string
		table      string
		conditions []modules.ListCondition
		want       []string
		wantErr    bool
	}{
		{
			name:       "sin condiciones devuelve la lista blanca ordenada",
			table:      "planes",
			conditions: nil,
			want:       []string{"eliminado", "id", "nombre"},
		},
		{
			name:  "condición sobre columna de lista blanca no duplica",
			table: "planes",
			conditions: []modules.ListCondition{
				{Field: "eliminado", Values: []string{"-1"}},
			},
			want: []string{"eliminado", "id", "nombre"},
		},
		{
			name:  "condición sobre columna de lista blanca que no se muestra se incluye",
			table: "planes",
			conditions: []modules.ListCondition{
				{Field: "eliminado", Values: []string{"-1"}},
			},
			want: []string{"eliminado", "id", "nombre"},
		},
		{
			name:  "columna de condición fuera de lista blanca aborta con ErrWhitelist",
			table: "planes",
			conditions: []modules.ListCondition{
				{Field: "secreto", Values: []string{"x"}},
			},
			wantErr: true,
		},
		{
			name:  "condición sin campo se ignora",
			table: "planes",
			conditions: []modules.ListCondition{
				{Field: "", Values: []string{"x"}},
			},
			want: []string{"eliminado", "id", "nombre"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectColumns(whitelist, tc.table, tc.conditions)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("selectColumns: se esperaba error, got nil")
				}
				if !errors.Is(err, ErrWhitelist) {
					t.Fatalf("selectColumns: el error debe envolver ErrWhitelist, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectColumns: error inesperado: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selectColumns\n  got:  %v\n  want: %v", got, tc.want)
			}
		})
	}
}
