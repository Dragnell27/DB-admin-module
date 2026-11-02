package modules

import (
	"fmt"
	"sort"
)

// RelationTree representa las tablas del módulo como un GRAFO DIRIGIDO por la
// dirección de la FK (ChildTable→ParentTable). Para la validación se exige que
// sea un DAG (dirigido, sin ciclos FK) y conexo (tratado como no dirigido). La
// relación padre/hijo la determina ChildTable→ParentTable de cada Relationship,
// NUNCA la distancia a la principal; la principal puede ocupar cualquier
// posición en el grafo.
type RelationTree struct {
	Principal string
	// FKParent[child] = padres según la FK (el hijo referencia al padre). Rige el
	// orden de borrado (hijo antes que padre).
	FKParent map[string][]string
	// FKChildren[parent] = hijos que referencian a parent por una FK.
	FKChildren map[string][]string
	Tables     []string // todas, orden estable
}

// BuildRelationTree construye el árbol a partir de la MultiTableSpec usando la
// dirección FK de cada relación. Devuelve error (en español, envuelto con
// ErrMultiTableValidation) si: una relación referencia una tabla no declarada;
// el grafo NO es conexo (toda tabla declarada debe ser alcanzable ignorando la
// dirección); hay un CICLO siguiendo la dirección FK. NO toca la BD. La
// principal NO está obligada a ser raíz.
func BuildRelationTree(spec MultiTableSpec) (RelationTree, error) {
	tables := spec.Tables()
	declared := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		declared[t] = struct{}{}
	}

	tree := RelationTree{
		Principal:  spec.PrincipalTable,
		FKParent:   make(map[string][]string),
		FKChildren: make(map[string][]string),
		Tables:     tables,
	}

	// Aristas no dirigidas para la conectividad.
	undirected := make(map[string]map[string]struct{}, len(tables))
	addUndirected := func(a, b string) {
		if undirected[a] == nil {
			undirected[a] = make(map[string]struct{})
		}
		if undirected[b] == nil {
			undirected[b] = make(map[string]struct{})
		}
		undirected[a][b] = struct{}{}
		undirected[b][a] = struct{}{}
	}

	for _, r := range spec.Relationships {
		if _, ok := declared[r.ChildTable]; !ok {
			return RelationTree{}, fmt.Errorf("%w: la relación referencia la tabla hija %q no declarada", ErrMultiTableValidation, r.ChildTable)
		}
		if _, ok := declared[r.ParentTable]; !ok {
			return RelationTree{}, fmt.Errorf("%w: la relación referencia la tabla padre %q no declarada", ErrMultiTableValidation, r.ParentTable)
		}
		if r.ChildTable == r.ParentTable {
			return RelationTree{}, fmt.Errorf("%w: la tabla %q no puede referenciarse a sí misma", ErrMultiTableValidation, r.ChildTable)
		}
		tree.FKParent[r.ChildTable] = appendUnique(tree.FKParent[r.ChildTable], r.ParentTable)
		tree.FKChildren[r.ParentTable] = appendUnique(tree.FKChildren[r.ParentTable], r.ChildTable)
		addUndirected(r.ChildTable, r.ParentTable)
	}

	// Conectividad: toda tabla declarada alcanzable desde la principal por aristas
	// no dirigidas. Solo exigible si hay más de una tabla.
	if len(tables) > 1 {
		if spec.PrincipalTable == "" {
			return RelationTree{}, fmt.Errorf("%w: el módulo multi-tabla no declara tabla principal", ErrMultiTableValidation)
		}
		reached := make(map[string]struct{})
		stack := []string{spec.PrincipalTable}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, ok := reached[cur]; ok {
				continue
			}
			reached[cur] = struct{}{}
			neighbors := make([]string, 0, len(undirected[cur]))
			for n := range undirected[cur] {
				neighbors = append(neighbors, n)
			}
			sort.Strings(neighbors)
			stack = append(stack, neighbors...)
		}
		for _, t := range tables {
			if _, ok := reached[t]; !ok {
				return RelationTree{}, fmt.Errorf("%w: la tabla %q no está enlazada por ninguna relación con la principal", ErrMultiTableValidation, t)
			}
		}
	}

	// Aciclicidad siguiendo la dirección FK (ChildTable→ParentTable).
	if err := tree.checkAcyclic(); err != nil {
		return RelationTree{}, err
	}

	// Ordena las listas de adyacencia para un recorrido determinista.
	for k := range tree.FKParent {
		sort.Strings(tree.FKParent[k])
	}
	for k := range tree.FKChildren {
		sort.Strings(tree.FKChildren[k])
	}
	return tree, nil
}

// checkAcyclic detecta un ciclo siguiendo las aristas dirigidas child→parent
// (FKParent). Un ciclo FK impediría derivar un orden de borrado válido.
func (t RelationTree) checkAcyclic() error {
	const (
		white = 0 // sin visitar
		gray  = 1 // en la pila de recursión
		black = 2 // terminado
	)
	color := make(map[string]int, len(t.Tables))
	var visit func(node string) error
	visit = func(node string) error {
		color[node] = gray
		parents := append([]string(nil), t.FKParent[node]...)
		sort.Strings(parents)
		for _, p := range parents {
			switch color[p] {
			case gray:
				return fmt.Errorf("%w: las relaciones FK forman un ciclo (p. ej. %q → %q)", ErrMultiTableValidation, node, p)
			case white:
				if err := visit(p); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}
	for _, tbl := range t.Tables {
		if color[tbl] == white {
			if err := visit(tbl); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteOrder devuelve, para una "tabla tope", la lista ORDENADA hijo→padre del
// conjunto de borrado: el subgrafo inducido por `top` y TODOS sus descendientes
// siguiendo FKChildren (las tablas que referencian a `top`, directa o
// transitivamente) MÁS la propia `top`. Los PADRES FK de `top` NO se incluyen.
// El orden es un orden topológico del subgrafo dirigido por FK (hijos primero,
// `top` la última), con desempate alfabético estable. Error si topTable no está
// en el árbol.
//
// Ejemplo (P ← A ← B, A referencia P, B referencia A):
//
//	DeleteOrder("P") = [B, A, P]; DeleteOrder("A") = [B, A]; DeleteOrder("B") = [B]
func (t RelationTree) DeleteOrder(topTable string) ([]string, error) {
	inTree := false
	for _, tbl := range t.Tables {
		if tbl == topTable {
			inTree = true
			break
		}
	}
	if !inTree {
		return nil, fmt.Errorf("%w: la tabla tope %q no pertenece al módulo", ErrMultiTableValidation, topTable)
	}

	// Conjunto = top + todos sus descendientes-hijo (vía FKChildren).
	inSet := make(map[string]struct{})
	stack := []string{topTable}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := inSet[cur]; ok {
			continue
		}
		inSet[cur] = struct{}{}
		children := append([]string(nil), t.FKChildren[cur]...)
		sort.Strings(children)
		stack = append(stack, children...)
	}

	// Orden topológico: hijos (poseedores de la FK) ANTES que sus padres; `top`
	// la última. Se emite un nodo solo cuando todos sus hijos dentro del conjunto
	// ya fueron emitidos (DFS post-orden sobre FKChildren restringido al conjunto).
	visited := make(map[string]struct{})
	out := make([]string, 0, len(inSet))
	var emit func(node string)
	emit = func(node string) {
		if _, ok := visited[node]; ok {
			return
		}
		visited[node] = struct{}{}
		children := make([]string, 0)
		for _, ch := range t.FKChildren[node] {
			if _, ok := inSet[ch]; ok {
				children = append(children, ch)
			}
		}
		sort.Strings(children)
		for _, ch := range children {
			emit(ch)
		}
		out = append(out, node)
	}
	// Recorre en orden estable todas las tablas del conjunto como posibles raíces
	// para cubrir componentes y asegurar hijos antes que padres.
	roots := make([]string, 0, len(inSet))
	for tbl := range inSet {
		roots = append(roots, tbl)
	}
	sort.Strings(roots)
	for _, r := range roots {
		emit(r)
	}
	return out, nil
}

// appendUnique añade s a la lista si no está presente.
func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
