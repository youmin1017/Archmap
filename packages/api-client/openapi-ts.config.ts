import { defineConfig } from "@hey-api/openapi-ts"

export default defineConfig({
  input: "../../contracts/openapi.yaml",
  output: { path: "src/gen", clean: true },
  plugins: [
    "@hey-api/typescript",
    "@hey-api/client-fetch",
    "zod",
    { name: "@hey-api/sdk", validator: "zod" },
  ],
})
