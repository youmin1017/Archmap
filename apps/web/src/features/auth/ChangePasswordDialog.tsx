import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { errMessage } from "../draft/queries"
import { useChangePassword, useLogout } from "./queries"

export const MIN_PASSWORD = 12

/** Problem with a new password, or null when it is acceptable (the API re-checks). */
export function passwordProblem(next: string, confirm: string): string | null {
  if (next.length < MIN_PASSWORD) return `Use at least ${MIN_PASSWORD} characters.`
  if (next !== confirm) return "The two new passwords differ."
  return null
}

/**
 * Change the caller's local password. Forced (no way to dismiss, sign out instead) while
 * a maintainer-set password must be replaced; the API blocks everything else until then.
 */
export function ChangePasswordDialog({
  open,
  onOpenChange,
  forced = false,
}: {
  open: boolean
  onOpenChange?: (open: boolean) => void
  forced?: boolean
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !forced && onOpenChange?.(o)} disablePointerDismissal={forced}>
      <DialogContent className="sm:max-w-md" showCloseButton={!forced}>
        {open && <Form forced={forced} onDone={() => onOpenChange?.(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function Form({ forced, onDone }: { forced: boolean; onDone: () => void }) {
  const change = useChangePassword()
  const signOut = useLogout()
  const [current, setCurrent] = useState("")
  const [next, setNext] = useState("")
  const [confirm, setConfirm] = useState("")
  const [touched, setTouched] = useState({ next: false, confirm: false })

  const problem = passwordProblem(next, confirm)
  const canSubmit = current !== "" && problem === null && !change.isPending
  const showNext = touched.next && next.length < MIN_PASSWORD
  const showConfirm = touched.confirm && next !== confirm

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!canSubmit) return
        change.mutate({ current, next }, { onSuccess: onDone })
      }}
    >
      <DialogHeader>
        <DialogTitle>{forced ? "Choose a new password" : "Change password"}</DialogTitle>
        <DialogDescription>
          {forced
            ? "A maintainer set your password. Replace it with one only you know to continue."
            : "Your other sessions are signed out when the password changes."}
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="pw-current">{forced ? "Temporary password" : "Current password"}</Label>
        <Input
          id="pw-current"
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          autoFocus
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="pw-next">New password</Label>
        <Input
          id="pw-next"
          type="password"
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, next: true }))}
          aria-invalid={showNext}
          aria-describedby="pw-next-hint"
        />
        <p id="pw-next-hint" className={showNext ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
          At least {MIN_PASSWORD} characters. A passphrase of a few words works well.
        </p>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="pw-confirm">Repeat new password</Label>
        <Input
          id="pw-confirm"
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, confirm: true }))}
          aria-invalid={showConfirm}
          aria-describedby={showConfirm ? "pw-confirm-error" : undefined}
        />
        {showConfirm && (
          <p id="pw-confirm-error" className="text-xs text-destructive">
            The two new passwords differ.
          </p>
        )}
      </div>

      {change.isError && (
        <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {errMessage(change.error)}
        </p>
      )}

      <DialogFooter className="items-center">
        {!canSubmit && !change.isPending && (
          <span className="mr-auto hidden text-xs text-muted-foreground sm:inline">
            {current === "" ? "Enter your current password." : problem}
          </span>
        )}
        {forced ? (
          <Button type="button" variant="outline" onClick={() => signOut.mutate()} disabled={signOut.isPending}>
            Sign out
          </Button>
        ) : (
          <Button type="button" variant="outline" onClick={onDone}>
            Cancel
          </Button>
        )}
        <Button type="submit" disabled={!canSubmit}>
          Change password
        </Button>
      </DialogFooter>
    </form>
  )
}
