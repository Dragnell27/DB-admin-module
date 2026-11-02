package users

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestUser_TableName(t *testing.T) {
	if got := (User{}).TableName(); got != "users" {
		t.Errorf("TableName() = %q, want %q", got, "users")
	}
}

func TestIsValidRole(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{RoleAdmin, true},
		{RoleViewer, true},
		{"admin", true},
		{"viewer", true},
		{"superadmin", false},
		{"", false},
		{"Admin", false},
	}
	for _, tc := range cases {
		if got := IsValidRole(tc.role); got != tc.want {
			t.Errorf("IsValidRole(%q) = %v, want %v", tc.role, got, tc.want)
		}
	}
}

func TestNewRepository_NoEsNil(t *testing.T) {
	if NewRepository(&gorm.DB{}) == nil {
		t.Fatal("NewRepository devolvió nil")
	}
}

// Create valida sus entradas antes de tocar la base de datos, por lo que estos
// casos no requieren conexión: deben fallar sin ejecutar ninguna consulta.
func TestCreate_RechazaEntradasInvalidas(t *testing.T) {
	repo := &gormRepository{db: &gorm.DB{}}
	ctx := context.Background()

	t.Run("usuario nil", func(t *testing.T) {
		if err := repo.Create(ctx, nil); err == nil {
			t.Fatal("se esperaba error al crear usuario nil")
		}
	})

	t.Run("username vacío", func(t *testing.T) {
		err := repo.Create(ctx, &User{Username: "   ", Role: RoleAdmin})
		if err == nil {
			t.Fatal("se esperaba error con username vacío")
		}
	})

	t.Run("rol inválido", func(t *testing.T) {
		err := repo.Create(ctx, &User{Username: "u1", Role: "root"})
		if err == nil {
			t.Fatal("se esperaba error con rol inválido")
		}
	})
}

func TestErrNotFound_EsComparable(t *testing.T) {
	// Garantiza que los llamadores pueden usar errors.Is contra ErrNotFound.
	wrapped := errors.Join(ErrNotFound, errors.New("contexto"))
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("ErrNotFound debe ser detectable con errors.Is tras envolverlo")
	}
}
