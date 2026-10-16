import { useSyncExternalStore } from "react"

function subscribe(onChange: () => void) {
  const observer = new MutationObserver(onChange)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
  return () => observer.disconnect()
}

const get = () => (document.documentElement.classList.contains("dark") ? "dark" : "light")

/** Current theme ("light" | "dark"), following the `.dark` class set by ThemeToggle. */
export function useColorMode() {
  return useSyncExternalStore(subscribe, get, () => "light" as const)
}
