package modules

import (
	"errors"
	"reflect"
	"testing"
)

// threeLevelSpec arma P ← A ← B: A referencia P (A hijo FK de P), B referencia A
// (B hijo FK de A). P es la principal.
func threeLevelSpec() MultiTableSpec {
	return MultiTableSpec{
		PrincipalTable: "P",
		ForeignTables:  []ForeignTable{{Table: "A"}, {Table: "B"}},
		Relationships: []Relationship{
			{ChildTable: "A", ParentTable: "P", On: []JoinPair{{Child: "p_id", Parent: "id"}}, JoinType: "inner"},
			{ChildTable: "B", ParentTable: "A", On: []JoinPair{{Child: "a_id", Parent: "id"}}, JoinType: "inner"},
		},
	}
}

func TestBuildRelationTree_Valid(t *testing.T) {
	tree, err := BuildRelationTree(threeLevelSpec())
	if err != nil {
		t.Fatalf("árbol válido no debe fallar: %v", err)
	}
	if !reflect.DeepEqual(tree.FKParent["A"], []string{"P"}) {
		t.Fatalf("FKParent[A]: %v", tree.FKParent["A"])
	}
	if !reflect.DeepEqual(tree.FKChildren["P"], []string{"A"}) {
		t.Fatalf("FKChildren[P]: %v", tree.FKChildren["P"])
	}
}

func TestBuildRelationTree_Cycle(t *testing.T) {
	spec := MultiTableSpec{
		PrincipalTable: "P",
		ForeignTables:  []ForeignTable{{Table: "A"}},
		Relationships: []Relationship{
			{ChildTable: "A", ParentTable: "P", JoinType: "inner"},
			{ChildTable: "P", ParentTable: "A", JoinType: "inner"},
		},
	}
	_, err := BuildRelationTree(spec)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("un ciclo FK debe rechazarse con ErrMultiTableValidation: %v", err)
	}
}

func TestBuildRelationTree_Orphan(t *testing.T) {
	spec := MultiTableSpec{
		PrincipalTable: "P",
		ForeignTables:  []ForeignTable{{Table: "A"}, {Table: "huerfana"}},
		Relationships: []Relationship{
			{ChildTable: "A", ParentTable: "P", JoinType: "inner"},
		},
	}
	_, err := BuildRelationTree(spec)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("una tabla huérfana debe rechazarse: %v", err)
	}
}

func TestBuildRelationTree_UndeclaredTable(t *testing.T) {
	spec := MultiTableSpec{
		PrincipalTable: "P",
		ForeignTables:  []ForeignTable{{Table: "A"}},
		Relationships: []Relationship{
			{ChildTable: "A", ParentTable: "fantasma", JoinType: "inner"},
		},
	}
	_, err := BuildRelationTree(spec)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("relación a tabla no declarada debe rechazarse: %v", err)
	}
}

func TestDeleteOrder_ThreeLevels(t *testing.T) {
	tree, err := BuildRelationTree(threeLevelSpec())
	if err != nil {
		t.Fatalf("árbol: %v", err)
	}
	cases := map[string][]string{
		"P": {"B", "A", "P"},
		"A": {"B", "A"},
		"B": {"B"},
	}
	for top, want := range cases {
		got, err := tree.DeleteOrder(top)
		if err != nil {
			t.Fatalf("DeleteOrder(%q): %v", top, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("DeleteOrder(%q)=%v want %v", top, got, want)
		}
	}
}

// Principal como HIJO de una catálogo (foránea padre): la principal se borra
// ANTES que la catálogo. DeleteOrder(principal) NO incluye la catálogo padre.
func TestDeleteOrder_PrincipalIsChild(t *testing.T) {
	spec := MultiTableSpec{
		PrincipalTable: "pagos",
		ForeignTables:  []ForeignTable{{Table: "catalogo"}},
		Relationships: []Relationship{
			{ChildTable: "pagos", ParentTable: "catalogo", On: []JoinPair{{Child: "cat_id", Parent: "id"}}, JoinType: "inner"},
		},
	}
	tree, err := BuildRelationTree(spec)
	if err != nil {
		t.Fatalf("árbol: %v", err)
	}
	// Borrar hasta la principal: solo "pagos" (no toca la catálogo padre).
	got, err := tree.DeleteOrder("pagos")
	if err != nil {
		t.Fatalf("DeleteOrder(pagos): %v", err)
	}
	if !reflect.DeepEqual(got, []string{"pagos"}) {
		t.Fatalf("DeleteOrder(pagos)=%v, se esperaba solo [pagos]", got)
	}
	// Borrar hasta la catálogo: el hijo (pagos) ANTES que el padre (catalogo).
	got, err = tree.DeleteOrder("catalogo")
	if err != nil {
		t.Fatalf("DeleteOrder(catalogo): %v", err)
	}
	if !reflect.DeepEqual(got, []string{"pagos", "catalogo"}) {
		t.Fatalf("DeleteOrder(catalogo)=%v, se esperaba [pagos catalogo]", got)
	}
}

func TestDeleteOrder_UnknownTop(t *testing.T) {
	tree, err := BuildRelationTree(threeLevelSpec())
	if err != nil {
		t.Fatalf("árbol: %v", err)
	}
	if _, err := tree.DeleteOrder("inexistente"); err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("tope desconocido debe fallar: %v", err)
	}
}
