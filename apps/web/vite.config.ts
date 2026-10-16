import { defineConfig } from "vite"
import { devtools } from "@tanstack/devtools-vite"
import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// Until session auth exists the API identifies callers by a dev-only header (AUTH_DEV_HEADER=true on the API).
const devUser = process.env.DEV_USER ?? "viewer@archmap.local"

const config = defineConfig({
  resolve: { tsconfigPaths: true },
  plugins: [devtools(), tailwindcss(), tanstackStart(), viteReact()],
  server: {
    proxy: {
      "/api": {
        target: process.env.API_PROXY_TARGET ?? "http://localhost:8080",
        configure: (proxy) => {
          // Default dev user, unless the browser (user switcher) already chose one.
          proxy.on("proxyReq", (proxyReq, req) => {
            if (!req.headers["x-dev-user"]) proxyReq.setHeader("X-Dev-User", devUser)
          })
        },
      },
    },
  },
})

export default config
