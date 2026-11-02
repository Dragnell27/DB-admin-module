// Package migrations expone, embebidos en el binario, los archivos SQL de
// migración del esquema del admin-backend. Embeberlos evita que el servidor
// dependa del directorio de trabajo en tiempo de ejecución (relevante en el
// contenedor Docker) y permite aplicarlos al arrancar (design.md, tarea 2.2).
//
// Las migraciones siguen la convención de golang-migrate en los nombres de
// archivo (`{version}_{nombre}.up.sql` / `.down.sql`); aquí solo se exponen los
// archivos embebidos y el runner (internal/db) los interpreta.
package migrations

import "embed"

// Files contiene todos los archivos .sql de este directorio embebidos en el
// binario. internal/db los lee para aplicar las migraciones "up" en orden.
//
//go:embed *.sql
var Files embed.FS
