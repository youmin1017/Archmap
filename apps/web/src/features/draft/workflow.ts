import type { Change, Changeset, ChangesetStatus, Me } from "@archmap/api-client"

export type WorkflowAction = "submit" | "approve" | "reject" | "merge" | "abandon"

export type ActionState = { action: WorkflowAction; enabled: boolean; reason?: string }

/** Capabilities of the current user on a changeset (UI gating only; the API enforces the real rules). */
export function capabilities(me: Me | undefined, cs: Changeset | undefined) {
  const isAuthor = !!me && !!cs && me.id === cs.author_id
  const isMaintainer = me?.role === "maintainer"
  const canEditRole = me?.role === "editor" || isMaintainer
  const open = cs?.status === "draft"
  return {
    isAuthor,
    isMaintainer,
    /** May change the draft's content (author or maintainer, while it is a draft). */
    canEdit: !!open && canEditRole && (isAuthor || isMaintainer),
    /** May record conflict resolutions. */
    canResolve: (cs?.status === "draft" || cs?.status === "in_review") && canEditRole && (isAuthor || isMaintainer),
    canProtect: isMaintainer,
  }
}

/** Which workflow buttons to show for a status, and whether they are usable now. */
export function workflowActions(me: Me | undefined, cs: Changeset | undefined): ActionState[] {
  if (!me || !cs) return []
  const { isAuthor, isMaintainer } = capabilities(me, cs)
  const mayAct = (me.role === "editor" || isMaintainer) && (isAuthor || isMaintainer)
  const status: ChangesetStatus = cs.status

  if (status === "draft") {
    return [
      { action: "submit", enabled: mayAct, reason: mayAct ? undefined : "Only the author or a maintainer can submit" },
      {
        action: "abandon",
        enabled: mayAct,
        reason: mayAct ? undefined : "Only the author or a maintainer can abandon",
      },
    ]
  }
  if (status === "in_review") {
    const approved = cs.approved_by !== null
    return [
      { action: "approve", enabled: isMaintainer, reason: isMaintainer ? undefined : "Only a maintainer can approve" },
      { action: "reject", enabled: isMaintainer, reason: isMaintainer ? undefined : "Only a maintainer can reject" },
      {
        action: "merge",
        enabled: isMaintainer && approved,
        reason: !isMaintainer ? "Only a maintainer can merge" : approved ? undefined : "Approve it first",
      },
      {
        action: "abandon",
        enabled: mayAct,
        reason: mayAct ? undefined : "Only the author or a maintainer can abandon",
      },
    ]
  }
  return []
}

/** Undo / redo availability from the change log (undone batches form the redo stack). */
export function undoRedoState(changes: readonly Change[]) {
  return {
    canUndo: changes.some((c) => c.undone_at === null),
    canRedo: changes.some((c) => c.undone_at !== null),
  }
}

export const statusLabel: Record<ChangesetStatus, string> = {
  draft: "Draft",
  in_review: "In review",
  merged: "Merged",
  rejected: "Rejected",
  abandoned: "Abandoned",
}
