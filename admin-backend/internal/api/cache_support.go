package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/ott"
	"admin-backend/internal/ottcache"
	"admin-backend/internal/validation"
)

// nowRFC3339 devuelve el instante actual en formato RFC3339, usado en las
// respuestas de los endpoints de refresco de caché para informar al front
// cuándo se actualizaron los datos.
func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

// writeCachedPage ordena/pagina el conjunto completo en memoria y responde con
// el mismo envoltorio normalizado que las respuestas en vivo, de modo que el
// front no distinga el origen (caché vs OTT). Lo comparten los handlers de los
// recursos cacheables.
func writeCachedPage(c *gin.Context, records []ottcache.Record, pagination validation.Pagination, ordering validation.Ordering) {
	pageRecords, meta := ottcache.Page(records, ottcache.PageParams{
		Page:           pagination.Page,
		RecordsPerPage: pagination.RecordsPerPage,
		OrderBy:        ordering.OrderBy,
		Order:          ordering.Order,
	})

	data := make([]interface{}, len(pageRecords))
	for i, r := range pageRecords {
		data[i] = r
	}

	metaCopy := meta
	c.JSON(http.StatusOK, ott.NormalizedResponse{
		OK:         true,
		Data:       data,
		Pagination: &metaCopy,
		Message:    "",
	})
}
