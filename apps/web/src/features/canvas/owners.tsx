import { createContext, useContext, useMemo } from "react"
import type { Directory, Owner } from "@archmap/api-client"

import { useDirectory } from "../users/queries"

/** An owner as shown: user and group owners carry the live name from the directory. */
export type OwnerView = Owner & {
  /** A disabled user or a deleted group (shown muted; lint warns). */
  inactive: boolean
}

/** Resolves owners against the directory (ADR-069); named owners pass through. */
export function ownerViews(owners: readonly Owner[], directory: Directory | undefined): Map<string, OwnerView> {
  const users = new Map(directory?.users.map((u) => [u.id, u]))
  const groups = new Map(directory?.groups.map((g) => [g.id, g]))
  return new Map(
    owners.map((o): [string, OwnerView] => {
      if (o.kind === "user" && o.principal_id) {
        const u = users.get(o.principal_id)
        return [o.id, { ...o, name: u?.name ?? o.name, inactive: directory !== undefined && (!u || u.disabled) }]
      }
      if (o.kind === "group" && o.principal_id) {
        const g = groups.get(o.principal_id)
        // A group missing from the directory was deleted (until the directory loads, assume not).
        return [o.id, { ...o, name: g?.name ?? o.name, inactive: directory !== undefined && !g }]
      }
      return [o.id, { ...o, inactive: false }]
    })
  )
}

// The owners of the model a workspace shows (main, a draft or a version). Canvas
// cards, the layers view, the tree filter and the details panel read names from it.
const OwnersContext = createContext<ReadonlyMap<string, OwnerView>>(new Map())

export function OwnersProvider({ owners, children }: { owners: readonly Owner[]; children: React.ReactNode }) {
  const directory = useDirectory()
  const index = useMemo(() => ownerViews(owners, directory.data), [owners, directory.data])
  return <OwnersContext.Provider value={index}>{children}</OwnersContext.Provider>
}

export const useOwnerIndex = () => useContext(OwnersContext)
