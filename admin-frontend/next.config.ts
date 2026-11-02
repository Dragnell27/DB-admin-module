import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Salida "standalone": Next.js genera un servidor mínimo autocontenido
  // (.next/standalone) que permite una imagen de runtime pequeña sin instalar
  // node_modules completo. Ver admin-frontend/Dockerfile.
  output: "standalone",
};

export default nextConfig;
