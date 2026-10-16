import { defineConfig } from "vitest/config"

// Separate from vite.config.ts so unit tests do not load the TanStack Start plugins.
export default defineConfig({
  resolve: { tsconfigPaths: true },
  test: { environment: "node", include: ["src/**/*.test.ts"] },
})
