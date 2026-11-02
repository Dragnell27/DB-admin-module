package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/audit"
)

// AuditHandler expone la consulta de la bitácora de auditoría: GET /audit-logs,
// con filtros opcionales (object_id, resource, actor, action, from, to) y
// paginación. La ruta la protege RequirePermission(audit, ver) en el router.
type AuditHandler struct {
	repo audit.Repository
}

// NewAuditHandler construye un AuditHandler con el repositorio de auditoría.
func NewAuditHandler(repo audit.Repository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// auditPagination es la forma de los metadatos de paginación en la respuesta.
type auditPagination struct {
	TotalRecords   int64 `json:"totalRecords"`
	Page           int   `json:"page"`
	RecordsPerPage int   `json:"recordsPerPage"`
	TotalPages     int   `json:"totalPages"`
}

// auditListData es la carga útil de GET /audit-logs: las entradas y su
// paginación.
type auditListData struct {
	Items      []audit.Entry   `json:"items"`
	Pagination auditPagination `json:"pagination"`
}

// List maneja GET /audit-logs. Lee los filtros de la query, normaliza la
// paginación y devuelve las entradas ordenadas por fecha descendente.
func (h *AuditHandler) List(c *gin.Context) {
	page := auditAtoiDefault(c.Query("page"), 1)
	perPage := auditAtoiDefault(c.Query("records_per_page"), 20)
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	filter := audit.Filter{
		ObjectID: strings.TrimSpace(c.Query("object_id")),
		Resource: strings.TrimSpace(c.Query("resource")),
		Actor:    strings.TrimSpace(c.Query("actor")),
		Action:   strings.TrimSpace(c.Query("action")),
		From:     parseAuditDate(c.Query("from"), false),
		To:       parseAuditDate(c.Query("to"), true),
		Page:     page,
		PerPage:  perPage,
	}

	items, total, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo consultar la bitácora de auditoría")
		return
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	writeOK(c, http.StatusOK, auditListData{
		Items: items,
		Pagination: auditPagination{
			TotalRecords:   total,
			Page:           page,
			RecordsPerPage: perPage,
			TotalPages:     totalPages,
		},
	}, "")
}

// auditAtoiDefault convierte una cadena a entero; si está vacía o es inválida,
// devuelve el valor por defecto.
func auditAtoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseAuditDate interpreta una fecha de filtro. Acepta RFC3339 completo o solo
// fecha (YYYY-MM-DD). Para el extremo `to` (endOfDay=true) con solo fecha, se
// ajusta al final del día para que el rango sea inclusivo. Devuelve nil si la
// cadena está vacía o no es parseable.
func parseAuditDate(s string, endOfDay bool) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return &t
	}
	return nil
}
