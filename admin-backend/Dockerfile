# syntax=docker/dockerfile:1

# ============================================================
# admin-backend (BFF Go + Gin) — imagen de producción multi-stage
# - Stage builder: compila binarios estáticos para cmd/api y cmd/seed
#   con el toolchain de Go que exige go.mod (go 1.26.0).
# - Stage runtime: imagen mínima que ejecuta el binario como usuario
#   no-root. Las migraciones SQL van embebidas en el binario (ver
#   internal/db/migrate.go), por lo que no se copian al runtime.
# ============================================================

# ----- Stage 1: builder -----
# golang:1.26-alpine satisface la directiva `go 1.26.0` de go.mod.
FROM golang:1.26-alpine AS builder

# git no es estrictamente necesario (sin dependencias VCS directas), pero
# ca-certificates se reutiliza en runtime; aquí solo necesitamos el compilador.
WORKDIR /src

# Cachear la descarga de módulos: copiar primero los manifiestos.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copiar el resto del código fuente.
COPY . .

# Compilar binarios estáticos (CGO deshabilitado) para Linux.
# -trimpath y -ldflags "-s -w" reducen el tamaño del binario.
ENV CGO_ENABLED=0 \
    GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ----- Stage 2: runtime -----
# Imagen mínima distroless sin shell ni gestor de paquetes; incluye un
# usuario no-root (nonroot, uid 65532) y certificados CA.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app

# Copiar los binarios compilados.
COPY --from=builder /out/api /app/api
COPY --from=builder /out/seed /app/seed

# Ejecutar como usuario no-root (provisto por la imagen distroless).
USER nonroot:nonroot

# El servidor escucha en PORT (default 8081, ver internal/config).
EXPOSE 8081

# Entrypoint: servidor HTTP del BFF. El seed se ejecuta como comando
# alternativo: `docker run <image> /app/seed`.
ENTRYPOINT ["/app/api"]
