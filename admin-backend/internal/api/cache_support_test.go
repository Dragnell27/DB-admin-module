package api

import (
	"time"

	"admin-backend/internal/ottcache"
)

// newTestCache construye un caché de lecturas OTT para las pruebas de handlers.
// Usa un TTL corto; los conjuntos se cargan vía los Loader de cada handler, que
// recorren el *ott.Client dirigido al doble de prueba de cada test.
func newTestCache() *ottcache.Cache {
	return ottcache.New(time.Minute)
}
