import { defineConfig } from "vite"
import { devtools } from "@tanstack/devtools-vite"
import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

const config = defineConfig({
  resolve: { tsconfigPaths: true },
  plugins: [devtools(), tailwindcss(), tanstackStart(), viteReact()],
  server: {
    // Same origin for the browser: the session cookie and the API's cross-origin check just work.
    proxy: { "/api": { target: process.env.API_PROXY_TARGET ?? "http://localhost:8080" } },
  },
})

export default config
