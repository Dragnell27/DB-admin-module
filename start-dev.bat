@echo off
REM ============================================================
REM  Panel Admin - arranque de desarrollo (ambas apps, 1 comando)
REM
REM  Levanta el admin-backend (Go, :8081) y el admin-frontend
REM  (Next.js, :3000) juntos en ESTA MISMA ventana, con la salida
REM  de ambos entremezclada y etiquetada [backend]/[frontend].
REM  Un solo Ctrl+C detiene AMBOS (concurrently -k).
REM
REM  Requisitos: Docker Desktop corriendo (BD), Go y Node en PATH,
REM  y admin-frontend/node_modules instalado (npm install).
REM ============================================================

cd /d "%~dp0admin-frontend"

echo === Panel Admin: backend (:8081) + frontend (:3000) ===
echo Backend : http://localhost:8081/health
echo Frontend: http://localhost:3000
echo (Ctrl+C detiene ambos)
echo.

npm run dev:all
