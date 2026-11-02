package auth

// Este archivo contiene la lógica PURA de auto-protección de la administración
// (Req. 14). No realiza ninguna I/O: los handlers son responsables de contar en
// la base de datos los administradores activos y de materializar el estado antes
// de invocar estas funciones. De esta forma la decisión es determinista y
// verificable mediante property-based testing (ver Property 11).

// IsAdminPermissions indica si un conjunto de permisos efectivos habilita la
// administración del sistema (Req. 14.1).
//
// Criterio exacto elegido (pragmático): el conjunto habilita la administración
// si existe AL MENOS UN permiso cuyo Resource sea ResourceUsers Y AL MENOS UN
// permiso cuyo Resource sea ResourceRoles. No se exige una acción concreta
// (ver/crear/editar/eliminar) ni que ambos permisos pertenezcan al mismo país:
// basta con poder administrar usuarios y roles en algún país. La ausencia de uno
// de los dos recursos implica que el usuario no puede sostener la administración
// por sí solo, por lo que no cuenta como administrador a efectos de la
// salvaguarda.
//
// Es una función pura y nunca entra en pánico, incluso con el valor cero
// EffectivePermissions{}.
func IsAdminPermissions(e EffectivePermissions) bool {
	hasUsers := false
	hasRoles := false
	for _, p := range e.List() {
		switch p.Resource {
		case ResourceUsers:
			hasUsers = true
		case ResourceRoles:
			hasRoles = true
		}
		if hasUsers && hasRoles {
			return true
		}
	}
	return hasUsers && hasRoles
}

// AdminState modela el estado ya materializado del sistema que se necesita para
// decidir una operación de salvaguarda. Lo construye el handler a partir de
// conteos y consultas en el repositorio; esta estructura NO hace I/O.
//
// Campos:
//   - ActiveAdmins: número de usuarios ACTIVOS cuyos permisos efectivos habilitan
//     la administración (IsAdminPermissions == true) en el estado actual, ANTES de
//     aplicar la operación. Debe incluir, si procede, al usuario objetivo de la
//     operación.
//   - TargetIsActiveAdmin: indica si el usuario objetivo de la operación es, en el
//     estado actual, un administrador activo (es decir, está contado dentro de
//     ActiveAdmins). Para operaciones que no tienen un usuario objetivo concreto
//     (p. ej. editar/eliminar un rol evaluado solo por su impacto agregado) puede
//     dejarse en false y usarse únicamente ActiveAdmins con ResultingAdmins de la
//     operación.
type AdminState struct {
	// ActiveAdmins es la cantidad de administradores activos antes de la operación.
	ActiveAdmins int
	// TargetIsActiveAdmin indica si el usuario objetivo cuenta hoy como admin activo.
	TargetIsActiveAdmin bool
}

// AdminOpKind enumera los tipos de operación que pueden reducir la capacidad de
// administración del sistema y que, por tanto, están sujetos a la salvaguarda
// (Req. 14.1, 14.2, 14.3).
type AdminOpKind int

const (
	// OpDeactivateUser representa desactivar un Usuario_Admin (Req. 14.2).
	OpDeactivateUser AdminOpKind = iota
	// OpEditUserRoles representa editar los roles de un Usuario_Admin, lo que puede
	// hacerle ganar o perder los permisos de administración (Req. 14.3).
	OpEditUserRoles
	// OpEditRole representa editar un Rol, cambiando los permisos de todos los
	// usuarios que lo tienen asignado (Req. 14.1).
	OpEditRole
	// OpDeleteRole representa eliminar un Rol asignado, retirándolo de todos sus
	// usuarios (Req. 14.1).
	OpDeleteRole
)

// AdminOp modela la operación propuesta y su efecto agregado sobre el número de
// administradores activos. El handler calcula estos campos de forma pura a
// partir del resultado previsto de la operación.
//
// Campos:
//   - Kind: tipo de operación (ver AdminOpKind). Es informativo/semántico y
//     permite a los handlers y pruebas distinguir el caso; la decisión numérica se
//     toma sobre los campos de efecto.
//   - TargetRemainsAdminAfter: SOLO relevante cuando hay un usuario objetivo con
//     TargetIsActiveAdmin == true (p. ej. OpDeactivateUser u OpEditUserRoles).
//     Indica si, tras aplicar la operación, ESE usuario objetivo seguiría siendo un
//     administrador activo. Al desactivar siempre será false; al editar roles será
//     IsAdminPermissions de los permisos efectivos resultantes del usuario.
//   - ResultingActiveAdmins: número total de administradores activos que quedarían
//     DESPUÉS de aplicar la operación, para los casos de impacto agregado donde el
//     handler puede calcularlo directamente (típicamente OpEditRole y
//     OpDeleteRole, que afectan a varios usuarios a la vez). Cuando este valor es
//     mayor o igual a cero se usa como fuente de verdad del resultado; un valor
//     negativo indica "no calculado" y hace que la decisión se derive de
//     AdminState + TargetRemainsAdminAfter.
type AdminOp struct {
	// Kind es el tipo de operación propuesta.
	Kind AdminOpKind
	// TargetRemainsAdminAfter indica si el usuario objetivo seguiría siendo admin.
	TargetRemainsAdminAfter bool
	// ResultingActiveAdmins es el total de admins activos tras la operación, o < 0
	// si el handler no lo calculó directamente.
	ResultingActiveAdmins int
}

// WouldLeaveNoAdmin decide, de forma PURA, si aplicar `op` sobre `state` dejaría
// al sistema SIN ningún usuario activo con permisos de administración (Req. 14.1,
// 14.2, 14.3). Devuelve true cuando la operación debe ser RECHAZADA por la
// salvaguarda.
//
// Reglas de decisión:
//
//  1. Si op.ResultingActiveAdmins >= 0, se considera la fuente de verdad del
//     impacto agregado (p. ej. editar o eliminar un rol que afecta a varios
//     usuarios): la operación deja sin admin si y solo si ese total es 0.
//
//  2. En caso contrario (ResultingActiveAdmins < 0, "no calculado"), la decisión
//     se deriva del usuario objetivo:
//     - Si el objetivo NO era admin activo (TargetIsActiveAdmin == false), la
//       operación no reduce el número de admins y nunca deja sin administración.
//     - Si el objetivo SÍ era admin activo y seguiría siéndolo tras la operación
//       (TargetRemainsAdminAfter == true), tampoco se pierde administración.
//     - Si el objetivo era admin activo y dejaría de serlo
//       (TargetRemainsAdminAfter == false), el sistema queda sin administración
//       solo si no hay ningún OTRO admin activo, es decir, si ActiveAdmins <= 1.
//
// Es una función pura y nunca entra en pánico.
func WouldLeaveNoAdmin(state AdminState, op AdminOp) bool {
	// Caso 1: impacto agregado calculado directamente por el handler.
	if op.ResultingActiveAdmins >= 0 {
		return op.ResultingActiveAdmins == 0
	}

	// Caso 2: decisión derivada del usuario objetivo.
	if !state.TargetIsActiveAdmin {
		// El objetivo no contaba como admin: la operación no reduce la administración.
		return false
	}
	if op.TargetRemainsAdminAfter {
		// El objetivo sigue siendo admin tras la operación: no se pierde capacidad.
		return false
	}
	// El objetivo era el único admin activo y dejaría de serlo.
	return state.ActiveAdmins <= 1
}
