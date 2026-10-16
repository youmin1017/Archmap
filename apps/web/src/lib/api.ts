import { client } from "@archmap/api-client"

import { installDevUserHeader } from "./dev-user"

// Same-origin: the Vite dev server proxies /api to the Go API (see vite.config.ts).
client.setConfig({ baseUrl: "" })
installDevUserHeader()
