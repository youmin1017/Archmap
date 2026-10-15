import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { getHealth } from "@archmap/api-client"
import { useReducedMotion } from "motion/react"
import { Network } from "lucide-react"

import "@/lib/api"
import { BorderBeam } from "@/components/ui/border-beam"
import { ThemeToggle } from "@/components/theme-toggle"
import { TechIcon } from "@/components/tech-icon/tech-icon"

export const Route = createFileRoute("/")({ component: App })

function App() {
  const reducedMotion = useReducedMotion()
  const health = useQuery({
    queryKey: ["health"],
    // Responses are Zod-validated by the generated SDK; drift in the backend
    // contract surfaces here as a thrown error.
    queryFn: async () => {
      const { data } = await getHealth({ throwOnError: true })
      return data
    },
  })

  return (
    <main className="mx-auto flex min-h-svh max-w-2xl flex-col gap-8 p-6">
      <header className="flex items-center justify-between pt-6">
        <div className="flex items-center gap-2">
          <Network className="size-6 text-primary" aria-hidden />
          <h1 className="font-heading text-xl font-semibold">Archmap</h1>
        </div>
        <ThemeToggle />
      </header>

      <section className="relative overflow-hidden rounded-2xl border bg-card p-6 text-card-foreground shadow-sm">
        <h2 className="text-sm font-medium text-muted-foreground">API status</h2>
        <p className="mt-2 text-2xl font-semibold" aria-live="polite">
          {health.isPending && "Checking…"}
          {health.isError && "API unreachable"}
          {health.isSuccess && `API: ${health.data.status}`}
        </p>
        {health.isSuccess && <p className="mt-1 text-sm text-muted-foreground">version {health.data.version}</p>}
        {health.isError && (
          <p className="mt-1 text-sm text-destructive">{health.error.message}. Is `task dev-api` running?</p>
        )}
        {health.isSuccess && !reducedMotion && <BorderBeam size={120} duration={8} />}
      </section>

      <section className="flex items-center gap-4 text-muted-foreground">
        {["postgres", "go", "react", "docker", "k8s", "nginx"].map((t) => (
          <TechIcon key={t} technology={t} className="size-6" />
        ))}
      </section>
    </main>
  )
}
