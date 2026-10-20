import { createContext, useContext } from "react"

// Drill-down (ADR-071): the workspace provides how to focus the canvas on an element
// (it updates the URL), so panels can offer "Focus" without knowing the route.
const FocusActionContext = createContext<((id: string | null) => void) | undefined>(undefined)

export const FocusActionProvider = FocusActionContext.Provider

/** Focuses the canvas on an element (null = whole canvas); undefined where drill-down is not available. */
export const useFocusAction = () => useContext(FocusActionContext)
