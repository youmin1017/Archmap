import { describe, expect, it } from "vitest"
import type { Change, Changeset, Me } from "@archmap/api-client"

import { capabilities, undoRedoState, workflowActions } from "./workflow"

const user = (role: Me["role"], id: string = role): Me => ({
  id,
  email: `${role}@x`,
  name: role,
  role,
  provider: "local",
  password_must_change: false,
})
const cs = (over: Partial<Changeset> = {}): Changeset => ({
  id: "c1",
  title: "t",
  description: "",
  author_id: "editor",
  base_version: 1,
  status: "draft",
  created_at: "2030-01-01T00:00:00Z",
  merged_at: null,
  approved_by: null,
  approved_at: null,
  kind: "draft",
  reverts: null,
  ...over,
})
const enabled = (me: Me, c: Changeset) => Object.fromEntries(workflowActions(me, c).map((a) => [a.action, a.enabled]))

describe("capabilities", () => {
  it("lets the author and maintainers edit an open draft, nobody else", () => {
    expect(capabilities(user("editor"), cs()).canEdit).toBe(true)
    expect(capabilities(user("maintainer"), cs()).canEdit).toBe(true)
    expect(capabilities(user("editor", "other"), cs()).canEdit).toBe(false)
    expect(capabilities(user("viewer", "editor"), cs()).canEdit).toBe(false)
    expect(capabilities(user("editor"), cs({ status: "in_review" })).canEdit).toBe(false)
    expect(capabilities(undefined, cs()).canEdit).toBe(false)
  })

  it("only maintainers may protect elements", () => {
    expect(capabilities(user("maintainer"), cs()).canProtect).toBe(true)
    expect(capabilities(user("editor"), cs()).canProtect).toBe(false)
  })

  it("allows conflict resolution while a changeset is open", () => {
    expect(capabilities(user("editor"), cs({ status: "in_review" })).canResolve).toBe(true)
    expect(capabilities(user("editor"), cs({ status: "merged" })).canResolve).toBe(false)
  })
})

describe("workflowActions", () => {
  it("drafts: submit and abandon for the author", () => {
    expect(enabled(user("editor"), cs())).toEqual({ submit: true, abandon: true })
    expect(enabled(user("editor", "other"), cs())).toEqual({ submit: false, abandon: false })
    expect(enabled(user("viewer", "editor"), cs())).toEqual({ submit: false, abandon: false })
  })

  it("in review: maintainers approve / reject, merge needs approval", () => {
    const review = cs({ status: "in_review" })
    expect(enabled(user("maintainer"), review)).toEqual({ approve: true, reject: true, merge: false, abandon: true })
    const approved = cs({ status: "in_review", approved_by: "maintainer", approved_at: "2030-01-02T00:00:00Z" })
    expect(enabled(user("maintainer"), approved).merge).toBe(true)
    expect(enabled(user("editor"), approved)).toEqual({ approve: false, reject: false, merge: false, abandon: true })
  })

  it("explains disabled actions", () => {
    const merge = workflowActions(user("maintainer"), cs({ status: "in_review" })).find((a) => a.action === "merge")
    expect(merge?.reason).toBe("Approve it first")
  })

  it("has no actions for finished changesets", () => {
    for (const status of ["merged", "rejected", "abandoned"] as const) {
      expect(workflowActions(user("maintainer"), cs({ status }))).toEqual([])
    }
  })
})

describe("undoRedoState", () => {
  const change = (undone: boolean): Change =>
    ({ id: "x", undone_at: undone ? "2030-01-01T00:00:00Z" : null }) as unknown as Change

  it("derives availability from the log", () => {
    expect(undoRedoState([])).toEqual({ canUndo: false, canRedo: false })
    expect(undoRedoState([change(false)])).toEqual({ canUndo: true, canRedo: false })
    expect(undoRedoState([change(false), change(true)])).toEqual({ canUndo: true, canRedo: true })
    expect(undoRedoState([change(true)])).toEqual({ canUndo: false, canRedo: true })
  })
})
