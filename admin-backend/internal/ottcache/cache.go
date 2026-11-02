// Package ottcache implementa un caché en memoria del BFF para los listados de
// lectura de OTT (customers, plans, subscriptions). Guarda el conjunto COMPLETO
// de cada (recurso, país) obtenido recorriendo todas las páginas de OTT, de modo
// que la paginación, el ordenamiento y los filtros propios del BFF se resuelvan
// en memoria sin golpear OTT en cada petición.
//
// El caché reside en memoria del proceso: no es persistente y una entrada
// perdida al reiniciar se reconstruye en la siguiente lectura. El acceso es
// seguro para concurrencia. Cada entrada expira por TTL; además puede
// invalidarse (tras una escritura) o refrescarse a mano (botón "Recargar").
//
// Este paquete no conoce Gin ni el contrato HTTP del frontend: solo almacena
// registros genéricos y delega la obtención desde OTT en un Loader que provee
// cada handler (que sí sabe su ruta/consulta OTT).
package ottcache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Resource identifica un recurso cacheable. El catálogo es cerrado: solo estos
// tres recursos se cachean (cards se consulta siempre en vivo).
type Resource string

const (
	ResourceCustomers     Resource = "customers"
	ResourcePlans         Resource = "plans"
	ResourceSubscriptions Resource = "subscriptions"
)

// Record es un registro individual del conjunto, un objeto JSON genérico de OTT
// (los campos varían por recurso; el BFF los trata de forma genérica).
type Record = map[string]interface{}

// Entry es el conjunto completo cacheado de un (recurso, país) junto con el
// instante en que se cargó, usado para evaluar el TTL.
type Entry struct {
	Records  []Record
	LoadedAt time.Time
}

// Loader obtiene el conjunto COMPLETO de un recurso+país desde OTT recorriendo
// todas sus páginas. Lo implementa cada handler. Debe ser atómico desde el punto
// de vista del llamador: devuelve el conjunto completo o un error, nunca un
// conjunto parcial.
type Loader func(ctx context.Context, country string) ([]Record, error)

// Cache es el almacén en memoria seguro para concurrencia, con expiración por
// TTL. El valor cero NO es utilizable; usar New.
type Cache struct {
	ttl  time.Duration
	now  func() time.Time
	mu   sync.Mutex
	data map[string]Entry
}

// New construye un Cache con el TTL dado. Un ttl no positivo se trata como 10
// minutos por seguridad (aunque la config ya garantiza un valor válido).
func New(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Cache{
		ttl:  ttl,
		now:  time.Now,
		data: make(map[string]Entry),
	}
}

// key compone la clave interna del caché a partir del recurso y el país
// (normalizado a minúsculas y sin espacios), de modo que (customers, gt) sea
// independiente de (customers, ni) y de (plans, gt).
func key(resource Resource, country string) string {
	return string(resource) + "|" + strings.ToLower(strings.TrimSpace(country))
}

// Get devuelve el conjunto completo vigente del (recurso, país). Si hay una
// entrada no vencida, la devuelve sin tocar OTT. Si no existe o venció, invoca
// loader, almacena el resultado y lo devuelve.
//
// La carga es ATÓMICA: si loader devuelve error, el caché no se modifica (ni se
// crea una entrada nueva ni se corrompe una previa) y se propaga el error.
//
// Devuelve una copia defensiva del slice (no del contenido de cada Record) para
// que el llamador pueda ordenar/recortar sin alterar el conjunto cacheado.
func (c *Cache) Get(ctx context.Context, resource Resource, country string, loader Loader) ([]Record, error) {
	k := key(resource, country)

	c.mu.Lock()
	entry, ok := c.data[k]
	fresh := ok && c.now().Sub(entry.LoadedAt) < c.ttl
	if fresh {
		records := entry.Records
		c.mu.Unlock()
		return cloneSlice(records), nil
	}
	c.mu.Unlock()

	// Cargar fuera del lock para no bloquear otras claves mientras se recorren
	// las páginas de OTT. Puede haber una carga redundante si dos peticiones del
	// mismo recurso+país coinciden; es aceptable y nunca produce estado corrupto.
	records, err := loader(ctx, country)
	if err != nil {
		return nil, fmt.Errorf("ottcache: no se pudo cargar %q (%s): %w", resource, country, err)
	}

	c.mu.Lock()
	c.data[k] = Entry{Records: records, LoadedAt: c.now()}
	c.mu.Unlock()

	return cloneSlice(records), nil
}

// Refresh fuerza recargar el conjunto desde OTT vía loader y reemplaza la
// entrada, sin importar si la actual está vigente. Devuelve el número de
// registros cargados. Si loader falla, el caché previo NO se toca y se propaga
// el error.
func (c *Cache) Refresh(ctx context.Context, resource Resource, country string, loader Loader) (int, error) {
	records, err := loader(ctx, country)
	if err != nil {
		return 0, fmt.Errorf("ottcache: no se pudo refrescar %q (%s): %w", resource, country, err)
	}

	k := key(resource, country)
	c.mu.Lock()
	c.data[k] = Entry{Records: records, LoadedAt: c.now()}
	c.mu.Unlock()

	return len(records), nil
}

// Invalidate elimina la entrada del (recurso, país), forzando que la siguiente
// lectura recargue desde OTT. Es seguro invalidar una clave inexistente.
func (c *Cache) Invalidate(resource Resource, country string) {
	k := key(resource, country)
	c.mu.Lock()
	delete(c.data, k)
	c.mu.Unlock()
}

// cloneSlice devuelve una copia superficial del slice de registros. Copia los
// punteros a cada Record (no clona los mapas), suficiente para que el llamador
// reordene o recorte páginas sin alterar el orden del conjunto cacheado.
func cloneSlice(records []Record) []Record {
	out := make([]Record, len(records))
	copy(out, records)
	return out
}
