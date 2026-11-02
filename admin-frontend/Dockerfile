# syntax=docker/dockerfile:1

# ============================================================
# admin-frontend (Next.js 15 + TypeScript) — imagen de producción multi-stage
# Usa la salida "standalone" de Next.js (output: "standalone" en
# next.config.ts) para un runtime pequeño que solo copia el servidor
# autocontenido y los assets estáticos.
#
# NOTA sobre NEXT_PUBLIC_ADMIN_API_URL: las variables NEXT_PUBLIC_* se
# incrustan en el bundle durante `next build`, por lo que se pasa como
# build-arg. Si cambia la URL del BFF, hay que reconstruir la imagen.
# ============================================================

# ----- Stage 1: deps -----
# Instala dependencias de forma reproducible con el package-lock.
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# ----- Stage 2: builder -----
FROM node:22-alpine AS builder
WORKDIR /app

# URL del BFF embebida en el bundle del cliente (NEXT_PUBLIC_*).
ARG NEXT_PUBLIC_ADMIN_API_URL=http://localhost:8081
ENV NEXT_PUBLIC_ADMIN_API_URL=${NEXT_PUBLIC_ADMIN_API_URL}

# Deshabilitar telemetría de Next.js durante el build.
ENV NEXT_TELEMETRY_DISABLED=1

COPY --from=deps /app/node_modules ./node_modules
COPY . .

RUN npm run build

# ----- Stage 3: runtime -----
FROM node:22-alpine AS runtime
WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

# Usuario no-root (el usuario `node` viene en la imagen oficial de Node).
# Copiar la salida standalone y los assets estáticos con la propiedad correcta.
COPY --from=builder --chown=node:node /app/public ./public
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static

USER node

EXPOSE 3000

# El servidor standalone de Next.js se arranca con server.js.
CMD ["node", "server.js"]
