import { useState } from "react"
import { Check, Copy } from "lucide-react"
import type { Role } from "@archmap/api-client"

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
import { cn } from "@/lib/utils"
import { errMessage } from "../draft/queries"
import { MIN_PASSWORD } from "../auth/ChangePasswordDialog"
import { useCreateUser } from "./queries"
import { roles } from "./roles"

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
const card =
  "flex flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
const cardOn = "border-primary bg-primary/5 ring-3 ring-primary/15"

/** Create a user with a local password: chosen now, or generated and shown once. */
export function NewUserDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [created, setCreated] = useState<{ name: string; email: string; password: string } | null>(null)
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) setCreated(null)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        {open && !created && (
          <Form onCancel={() => onOpenChange(false)} onCreated={setCreated} onDone={() => onOpenChange(false)} />
        )}
        {open && created && (
          <OneTimePassword
            title={`${created.name} can sign in`}
            who={created.email}
            password={created.password}
            onDone={() => {
              setCreated(null)
              onOpenChange(false)
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function Form({
  onCancel,
  onCreated,
  onDone,
}: {
  onCancel: () => void
  onCreated: (v: { name: string; email: string; password: string }) => void
  onDone: () => void
}) {
  const create = useCreateUser()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<Role>("viewer")
  const [generate, setGenerate] = useState(true)
  const [password, setPassword] = useState("")
  const [touched, setTouched] = useState({ name: false, email: false, password: false })

  const emailOk = /^\S+@\S+$/.test(email.trim())
  const passwordOk = generate || password.length >= MIN_PASSWORD
  const missing = !name.trim()
    ? "Enter a name."
    : !emailOk
      ? "Enter a valid email."
      : !passwordOk
        ? `The password needs at least ${MIN_PASSWORD} characters.`
        : null
  const canSubmit = missing === null && !create.isPending

  return (
    <form
      className="flex max-h-[85vh] min-h-0 flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!canSubmit) return
        create.mutate(
          { name: name.trim(), email: email.trim(), role, ...(generate ? {} : { password }) },
          {
            onSuccess: (res) => {
              if (res.temporary_password) {
                onCreated({ name: res.user.name, email: res.user.email, password: res.temporary_password })
              } else {
                onDone()
              }
            },
          }
        )
      }}
    >
      <DialogHeader>
        <DialogTitle>New user</DialogTitle>
        <DialogDescription>The email is also how they sign in with a password.</DialogDescription>
      </DialogHeader>

      <div className="-mx-4 flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-1">
        <section className="flex flex-col gap-3" aria-labelledby="nu-account">
          <h3 id="nu-account" className={heading}>
            Account
          </h3>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="nu-name">Name</Label>
            <Input
              id="nu-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, name: true }))}
              aria-invalid={touched.name && !name.trim()}
              autoFocus
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="nu-email">Email</Label>
            <Input
              id="nu-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, email: true }))}
              aria-invalid={touched.email && !emailOk}
              aria-describedby="nu-email-hint"
            />
            <p
              id="nu-email-hint"
              className={cn("text-xs", touched.email && !emailOk ? "text-destructive" : "text-muted-foreground")}
            >
              {touched.email && !emailOk
                ? "Enter a valid email."
                : "Accounts are never matched by email to single sign-on."}
            </p>
          </div>
        </section>

        <fieldset className="flex flex-col gap-2">
          <legend className={cn(heading, "mb-2")}>Role</legend>
          <div role="radiogroup" aria-label="Role" className="grid gap-2 sm:grid-cols-3">
            {roles.map((r) => (
              <button
                key={r.value}
                type="button"
                role="radio"
                aria-checked={role === r.value}
                onClick={() => setRole(r.value)}
                className={cn(card, role === r.value ? cardOn : "hover:bg-muted/60")}
              >
                <span className="text-sm font-medium">{r.label}</span>
                <span className="text-xs text-muted-foreground">{r.short}</span>
              </button>
            ))}
          </div>
        </fieldset>

        <fieldset className="flex flex-col gap-2">
          <legend className={cn(heading, "mb-2")}>Password</legend>
          <div role="radiogroup" aria-label="Password" className="grid gap-2 sm:grid-cols-2">
            <button
              type="button"
              role="radio"
              aria-checked={generate}
              onClick={() => setGenerate(true)}
              className={cn(card, generate ? cardOn : "hover:bg-muted/60")}
            >
              <span className="text-sm font-medium">Generate</span>
              <span className="text-xs text-muted-foreground">Shown once; they change it at first sign-in</span>
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={!generate}
              onClick={() => setGenerate(false)}
              className={cn(card, !generate ? cardOn : "hover:bg-muted/60")}
            >
              <span className="text-sm font-medium">Set now</span>
              <span className="text-xs text-muted-foreground">You hand it over yourself</span>
            </button>
          </div>
          {!generate && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="nu-password">Password</Label>
              <Input
                id="nu-password"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, password: true }))}
                aria-invalid={touched.password && !passwordOk}
                aria-describedby="nu-password-hint"
              />
              <p
                id="nu-password-hint"
                className={cn(
                  "text-xs",
                  touched.password && !passwordOk ? "text-destructive" : "text-muted-foreground"
                )}
              >
                At least {MIN_PASSWORD} characters.
              </p>
            </div>
          )}
        </fieldset>

        {create.isError && (
          <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {errMessage(create.error)}
          </p>
        )}
      </div>

      <DialogFooter className="items-center">
        {missing && !create.isPending && (
          <span className="mr-auto hidden text-xs text-muted-foreground sm:inline">{missing}</span>
        )}
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={!canSubmit}>
          Create user
        </Button>
      </DialogFooter>
    </form>
  )
}

/** A generated password, shown exactly once with a copy button. */
export function OneTimePassword({
  title,
  who,
  password,
  onDone,
}: {
  title: string
  who: string
  password: string
  onDone: () => void
}) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>
          Give this temporary password to {who || "the user"}. It is shown only now; they must replace it when they sign
          in.
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-2">
        <code className="min-w-0 flex-1 truncate px-1 font-mono text-sm" aria-label="Temporary password">
          {password}
        </code>
        <Button
          size="sm"
          variant="outline"
          onClick={() => {
            navigator.clipboard.writeText(password).then(
              () => setCopied(true),
              () => setCopied(false)
            )
          }}
        >
          {copied ? <Check aria-hidden /> : <Copy aria-hidden />} {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <DialogFooter>
        <Button onClick={onDone}>Done</Button>
      </DialogFooter>
    </div>
  )
}
