import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { FilePlus2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useCreateChangeset, useMe } from "./queries"

/** Button + dialog that opens a new draft (editors and maintainers) and navigates to it. */
export function NewDraftButton({ variant = "default" }: { variant?: "default" | "outline" }) {
  const me = useMe()
  const [open, setOpen] = useState(false)
  const canCreate = me.data?.role === "editor" || me.data?.role === "maintainer"

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            variant={variant}
            size="sm"
            disabled={!canCreate}
            title={canCreate ? undefined : "Viewers cannot create drafts"}
          />
        }
      >
        <FilePlus2 aria-hidden /> New draft
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">{open && <Form onClose={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  )
}

function Form({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const create = useCreateChangeset()
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!title.trim() || create.isPending) return
        create.mutate(
          { title: title.trim(), description },
          {
            onSuccess: (cs) => {
              onClose()
              void navigate({ to: "/changesets/$id", params: { id: cs.id } })
            },
          }
        )
      }}
    >
      <DialogHeader>
        <DialogTitle>New draft</DialogTitle>
        <DialogDescription>
          All edits happen in a draft. Main only changes when a maintainer merges it.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="draft-title">Title</Label>
        <Input
          id="draft-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
          placeholder="e.g. Add a cache in front of the API"
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="draft-description">Description (optional)</Label>
        <Textarea
          id="draft-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={3}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={!title.trim() || create.isPending}>
          Create draft
        </Button>
      </DialogFooter>
    </form>
  )
}
