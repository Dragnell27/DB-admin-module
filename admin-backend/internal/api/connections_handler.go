package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/connections"
	"admin-backend/internal/engine"
)

// ConnectionsHandler expone el CRUD de plantillas de conexión
// (connection-templates). Nunca devuelve los valores FIJOS ni el DSN efectivo.
// Tras editar una plantilla invalida los pools de los módulos afectados (D-4).
type ConnectionsHandler struct {
	repo        connections.Repository
	invalidator engine.PoolInvalidator
}

// NewConnectionsHandler construye el handler con el repositorio de plantillas y
// el invalidador de pools (puede ser nil: no-op).
func NewConnectionsHandler(repo connections.Repository, inv engine.PoolInvalidator) *ConnectionsHandler {
	return &ConnectionsHandler{repo: repo, invalidator: inv}
}

// connectionTemplateBody es el cuerpo de crear/editar de una plantilla. Los
// valores FIJOS viajan bajo la clave de primer nivel `fixedValues` para que la
// redacción de auditoría por clave contenedora los elimine de la bitácora.
type connectionTemplateBody struct {
	Name        string                       `json:"name"`
	Fields      []connections.TemplateField  `json:"fields"`
	FixedValues map[string]string            `json:"fixedValues"`
}

// List maneja GET /connection-templates.
func (h *ConnectionsHandler) List(c *gin.Context) {
	items, err := h.repo.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar las plantillas de conexión")
		return
	}
	writeOK(c, http.StatusOK, gin.H{"items": items}, "")
}

// Get maneja GET /connection-templates/:id.
func (h *ConnectionsHandler) Get(c *gin.Context) {
	tmpl, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeTemplateError(c, err)
		return
	}
	writeOK(c, http.StatusOK, tmpl, "")
}

// Create maneja POST /connection-templates.
func (h *ConnectionsHandler) Create(c *gin.Context) {
	var body connectionTemplateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Cuerpo de la petición inválido")
		return
	}
	tmpl, err := h.repo.Create(c.Request.Context(), body.Name, body.Fields, connections.FixedValues(body.FixedValues))
	if err != nil {
		h.writeTemplateError(c, err)
		return
	}
	SetAuditObjectID(c, tmpl.ID)
	writeOK(c, http.StatusCreated, gin.H{"id": tmpl.ID, "name": tmpl.Name}, "Plantilla de conexión creada")
}

// Update maneja PUT /connection-templates/:id. Tras un update exitoso invalida
// los pools de los módulos que usan la plantilla (D-4).
func (h *ConnectionsHandler) Update(c *gin.Context) {
	id := c.Param("id")
	SetAuditObjectID(c, id)
	var body connectionTemplateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Cuerpo de la petición inválido")
		return
	}
	tmpl, err := h.repo.Update(c.Request.Context(), id, body.Name, body.Fields, connections.FixedValues(body.FixedValues))
	if err != nil {
		h.writeTemplateError(c, err)
		return
	}
	// Invalidar los pools afectados para que la próxima ejecución reabra con el
	// DSN efectivo recalculado (D-4). nil invalidador = no-op.
	if h.invalidator != nil {
		slugs, serr := h.repo.SlugsUsingTemplate(c.Request.Context(), id)
		if serr == nil {
			for _, slug := range slugs {
				h.invalidator.Invalidate(slug)
			}
		}
	}
	writeOK(c, http.StatusOK, gin.H{"id": tmpl.ID, "name": tmpl.Name}, "Plantilla de conexión actualizada")
}

// Delete maneja DELETE /connection-templates/:id. No necesita invalidación
// (D-5 bloquea el borrado si hay módulos que la referencian).
func (h *ConnectionsHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	SetAuditObjectID(c, id)
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		h.writeTemplateError(c, err)
		return
	}
	writeOK(c, http.StatusOK, gin.H{"id": id}, "Plantilla de conexión eliminada")
}

// writeTemplateError traduce los errores de negocio del repositorio a respuestas
// según el diseño (Manejo de errores → Plantillas).
func (h *ConnectionsHandler) writeTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, connections.ErrNotFound):
		writeError(c, http.StatusNotFound, "E404", "Plantilla de conexión no encontrada")
	case errors.Is(err, connections.ErrNameTaken):
		writeError(c, http.StatusConflict, "E409", "El nombre de la plantilla ya está en uso")
	case errors.Is(err, connections.ErrTemplateInUse):
		count := h.usageCount(c, c.Param("id"))
		writeError(c, http.StatusConflict, "E409_TEMPLATE_IN_USE", fmt.Sprintf("%d módulos usan esta plantilla", count))
	case errors.Is(err, connections.ErrValidation):
		writeError(c, http.StatusUnprocessableEntity, "E422_VALIDATION", "La plantilla de conexión es inválida: "+err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo procesar la plantilla de conexión")
	}
}

// usageCount intenta obtener el conteo de módulos que usan la plantilla para el
// mensaje de conflicto; 0 si no se puede resolver.
func (h *ConnectionsHandler) usageCount(c *gin.Context, id string) int {
	tmpl, err := h.repo.Get(c.Request.Context(), id)
	if err != nil {
		return 0
	}
	return tmpl.UsedByModules
}
