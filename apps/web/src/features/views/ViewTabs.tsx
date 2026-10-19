import { useState } from "react"
import { Eye, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react"
import type { Element, Me, Model, View, ViewModel } from "@archmap/api-client"

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
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"
import { useCreateView, useDeleteView, useUpdateView, useViews } from "../draft/queries"
import { ViewDialog, viewKinds } from "./ViewDialog"

type Props = {
  me: Me | undefined
  activeId: string | undefined
  onSelect: (id: string | undefined) => void
  /** Every element of the model being shown (for the scope / member pickers). */
  model: Model | undefined
  /** The active view's data (members of a custom view are read from its layout). */
  activeModel: ViewModel | undefined
  /** Page actions shown at the right end of the bar (draft tools, New draft, ...). */
  actions?: React.ReactNode
}

const tab =
  "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2.5 text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none [&_svg]:size-3.5"
const tabOn = "bg-muted font-medium text-foreground"
const tabOff = "text-muted-foreground hover:bg-muted/60 hover:text-foreground"

/**
 * Tab strip above the canvas: the whole model plus saved views. Creating,
 * renaming and deleting views needs an editor; views never change the model.
 */
export function ViewTabs({ me, activeId, onSelect, model, activeModel, actions }: Props) {
  const views = useViews()
  const create = useCreateView()
  const update = useUpdateView()
  const remove = useDeleteView()

  const [dialog, setDialog] = useState<{ view?: View } | null>(null)
  const [deleting, setDeleting] = useState<View | null>(null)

  const canEditRole = me?.role === "editor" || me?.role === "maintainer"
  const mayManage = (v: View) => canEditRole && (v.created_by === me.id || me.role === "maintainer")
  const elements: readonly Element[] = model?.elements ?? []
  const active = views.data?.find((v) => v.id === activeId)
  const members = activeModel?.layout.filter((l) => l.included).map((l) => l.element_id)

  return (
    <div className="flex h-11 shrink-0 items-center gap-2 border-b bg-background px-2">
      <div className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto" role="tablist" aria-label="Views">
        <button
          type="button"
          role="tab"
          aria-selected={activeId === undefined}
          onClick={() => onSelect(undefined)}
          className={cn(tab, activeId === undefined ? tabOn : tabOff)}
        >
          <Eye aria-hidden /> Whole model
        </button>

        {views.data?.map((v) => {
          const KindIcon = viewKinds.find((k) => k.value === v.kind)?.icon ?? Eye
          return (
            <span key={v.id} className="inline-flex shrink-0 items-center">
              <button
                type="button"
                role="tab"
                aria-selected={v.id === activeId}
                title={`${viewKinds.find((k) => k.value === v.kind)?.label ?? v.kind}${v.description ? ` · ${v.description}` : ""}`}
                onClick={() => onSelect(v.id)}
                className={cn(tab, v.id === activeId ? tabOn : tabOff)}
              >
                <KindIcon aria-hidden /> {v.name}
              </button>
              {v.id === activeId && mayManage(v) && (
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={<Button variant="ghost" size="icon-xs" aria-label={`Manage view ${v.name}`} />}
                  >
                    <MoreHorizontal aria-hidden />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuItem onClick={() => setDialog({ view: v })}>
                      <Pencil aria-hidden /> Edit…
                    </DropdownMenuItem>
                    <DropdownMenuItem variant="destructive" onClick={() => setDeleting(v)}>
                      <Trash2 aria-hidden /> Delete…
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </span>
          )
        })}

        <Button
          variant="ghost"
          size="sm"
          className="text-muted-foreground"
          disabled={!canEditRole}
          title={canEditRole ? "Create a view" : "Viewers cannot create views"}
          onClick={() => setDialog({})}
          aria-label="New view"
        >
          <Plus aria-hidden /> New view
        </Button>
        {views.isError && <span className="text-xs text-destructive">Could not load views</span>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}

      <ViewDialog
        open={dialog !== null}
        onOpenChange={(o) => !o && setDialog(null)}
        elements={elements}
        view={dialog?.view}
        members={dialog?.view && dialog.view.id === activeId ? members : undefined}
        pending={create.isPending || update.isPending}
        onSubmit={(input) => {
          const existing = dialog?.view
          const done = () => setDialog(null)
          if (existing) update.mutate({ id: existing.id, body: input }, { onSuccess: done })
          else
            create.mutate(input, {
              onSuccess: (v) => {
                done()
                onSelect(v.id)
              },
            })
        }}
      />

      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete view “{deleting?.name}”?</AlertDialogTitle>
            <AlertDialogDescription>
              The view and its saved layout are removed. The model itself is not affected.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (!deleting) return
                const id = deleting.id
                remove.mutate(id, { onSuccess: () => id === active?.id && onSelect(undefined) })
                setDeleting(null)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
