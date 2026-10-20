import { useState } from "react"
import type { Identity, User } from "@archmap/api-client"

import { StatePanel } from "@/components/state-panel"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { errMessage } from "../draft/queries"
import { useAuthProviders } from "../auth/queries"
import { useIdentities, useLinkIdentity, useUnlinkIdentity } from "./queries"

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
const LOCAL_ISSUER = "urn:archmap:local"

function when(t?: string | null): string {
  return t ? new Date(t).toLocaleString() : "Never"
}

/** A user's sign-in identities. Linking is explicit: nothing is matched by email (ADR-059). */
export function IdentitiesDialog({ user, onClose }: { user: User | null; onClose: () => void }) {
  return (
    <Dialog open={user !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">{user && <Body user={user} />}</DialogContent>
    </Dialog>
  )
}

function Body({ user }: { user: User }) {
  const identities = useIdentities(user.id)
  const unlink = useUnlinkIdentity(user.id)
  const [removing, setRemoving] = useState<Identity | null>(null)

  return (
    <div className="flex max-h-[85vh] min-h-0 flex-col gap-4">
      <DialogHeader>
        <DialogTitle>Sign-in identities of {user.name}</DialogTitle>
        <DialogDescription>
          Each identity is one way to sign in. Removing one signs out the sessions started with it.
        </DialogDescription>
      </DialogHeader>

      <div className="-mx-4 flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-1">
        {identities.isPending && <StatePanel tone="loading">Loading identities…</StatePanel>}
        {identities.isError && (
          <StatePanel tone="error" title="Could not load identities" onRetry={() => void identities.refetch()}>
            {errMessage(identities.error)}
          </StatePanel>
        )}
        {identities.isSuccess && identities.data.length === 0 && (
          <StatePanel title="No identities">
            This user cannot sign in until a password is reset or an identity is linked.
          </StatePanel>
        )}
        {identities.isSuccess && identities.data.length > 0 && (
          <ul className="flex flex-col divide-y rounded-lg border">
            {identities.data.map((i) => (
              <li key={i.id} className="flex items-start gap-3 p-3">
                <dl className="grid min-w-0 flex-1 grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm">
                  <dt className="text-muted-foreground">Method</dt>
                  <dd className="font-medium">{i.issuer === LOCAL_ISSUER ? "Email and password" : i.provider}</dd>
                  <dt className="text-muted-foreground">Signs in as</dt>
                  <dd className="truncate font-mono text-xs leading-5" title={i.subject}>
                    {i.subject}
                  </dd>
                  {i.issuer !== LOCAL_ISSUER && (
                    <>
                      <dt className="text-muted-foreground">Issuer</dt>
                      <dd className="truncate text-xs leading-5">{i.issuer}</dd>
                    </>
                  )}
                  <dt className="text-muted-foreground">Last sign-in</dt>
                  <dd className="text-xs leading-5">{when(i.last_login_at)}</dd>
                </dl>
                <Button size="sm" variant="outline" onClick={() => setRemoving(i)} disabled={unlink.isPending}>
                  Remove
                </Button>
              </li>
            ))}
          </ul>
        )}

        <LinkForm user={user} />
      </div>

      <AlertDialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove this sign-in identity?</AlertDialogTitle>
            <AlertDialogDescription>
              {user.name} can no longer sign in with it, and sessions started with it end now.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (removing) unlink.mutate(removing.id)
                setRemoving(null)
              }}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

/** Link an external identity by its subject; only offered when such providers are configured. */
function LinkForm({ user }: { user: User }) {
  const providers = useAuthProviders()
  const link = useLinkIdentity(user.id)
  const external = (providers.data ?? []).filter((p) => p.kind === "redirect")
  const [provider, setProvider] = useState<string | null>(null)
  const [subject, setSubject] = useState("")
  if (external.length === 0) return null
  const chosen = provider ?? external[0].id
  const canSubmit = subject.trim() !== "" && !link.isPending

  return (
    <form
      className="flex flex-col gap-3"
      aria-labelledby="link-heading"
      onSubmit={(e) => {
        e.preventDefault()
        if (!canSubmit) return
        link.mutate({ provider: chosen, subject: subject.trim() }, { onSuccess: () => setSubject("") })
      }}
    >
      <h3 id="link-heading" className={heading}>
        Link an identity
      </h3>
      <div className="grid gap-2 sm:grid-cols-[10rem_1fr]">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="link-provider">Provider</Label>
          <Select
            value={chosen}
            onValueChange={(v) => v && setProvider(v)}
            items={external.map((p) => ({ value: p.id, label: p.name }))}
          >
            <SelectTrigger id="link-provider" size="sm" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {external.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="link-subject">Subject</Label>
          <Input
            id="link-subject"
            className="h-7"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            aria-describedby="link-subject-hint"
          />
        </div>
      </div>
      <p id="link-subject-hint" className="text-xs text-muted-foreground">
        The identity provider's stable user id (the <code>sub</code> claim), not an email.
      </p>
      {link.isError && (
        <p role="alert" className="text-xs text-destructive">
          {errMessage(link.error)}
        </p>
      )}
      <Button type="submit" size="sm" className="self-end" disabled={!canSubmit}>
        Link identity
      </Button>
    </form>
  )
}
