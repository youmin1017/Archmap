import { describe, expect, it } from "vitest"
import type { Directory, Owner } from "@archmap/api-client"

import { principalOwnerId } from "@/lib/uuidv5"
import { ownerViews } from "../canvas/owners"
import { ownerSections, principalOwnerInputs } from "./ownerPicker"

const ada = "00000000-0000-7000-8000-0000000000a1"
const bob = "00000000-0000-7000-8000-0000000000b1"
const gone = "00000000-0000-7000-8000-0000000000c1"
const pay = "00000000-0000-7000-8000-0000000000d1"

const directory: Directory = {
  users: [
    { id: ada, name: "Ada", email: "ada@shop.example", disabled: false },
    { id: bob, name: "Bob", email: "bob@shop.example", disabled: true },
  ],
  groups: [{ id: pay, name: "Payments", description: "", members: [ada], created_at: "2026-01-01T00:00:00Z" }],
}

const owner = (o: Partial<Owner> & Pick<Owner, "id" | "kind" | "name">): Owner => ({
  contact: "",
  oncall_url: "",
  principal_id: null,
  version: 1,
  ...o,
})

const team = owner({ id: "t1", kind: "team", name: "Core" })
const adaOwner = owner({ id: principalOwnerId(ada), kind: "user", name: "Ada L.", principal_id: ada })
const goneOwner = owner({ id: principalOwnerId(gone), kind: "group", name: "Old group", principal_id: gone })

describe("ownerSections", () => {
  it("lists users and groups from the directory and named owners as teams", () => {
    const s = ownerSections([team, adaOwner], directory)
    expect(s.map((x) => x.label)).toEqual(["Users", "Groups", "Teams"])
    // The user owner already in the model is the same entry as the directory user (live name).
    expect(s[0].choices).toEqual([
      expect.objectContaining({ value: principalOwnerId(ada), label: "Ada", detail: "ada@shop.example" }),
    ])
    expect(s[1].choices[0]).toMatchObject({ value: principalOwnerId(pay), kind: "group", principalId: pay })
    expect(s[2].choices).toEqual([expect.objectContaining({ value: "t1", label: "Core" })])
  })

  it("hides disabled users and deleted groups unless already assigned", () => {
    expect(
      ownerSections([goneOwner], directory)
        .flatMap((x) => x.choices)
        .map((c) => c.label)
    ).toEqual(["Ada", "Payments"])
    const kept = ownerSections([goneOwner], directory, [principalOwnerId(bob), goneOwner.id]).flatMap((x) => x.choices)
    expect(kept.filter((c) => c.inactive).map((c) => c.label)).toEqual(["Bob", "Old group"])
  })
})

describe("principalOwnerInputs", () => {
  const sections = ownerSections([team, adaOwner], directory)

  it("adds the owner for a user or group picked for the first time, once", () => {
    const inputs = principalOwnerInputs(
      [principalOwnerId(pay), principalOwnerId(pay), "", "t1"],
      [team, adaOwner],
      sections
    )
    expect(inputs).toEqual([
      {
        op: "add",
        target_type: "owner",
        patch: { kind: "group", name: "", contact: "", oncall_url: "", principal_id: pay },
      },
    ])
  })

  it("adds nothing for owners the model already has", () => {
    expect(principalOwnerInputs([principalOwnerId(ada), "t1"], [team, adaOwner], sections)).toEqual([])
  })
})

describe("ownerViews", () => {
  it("shows live names and marks disabled users and deleted groups", () => {
    const bobOwner = owner({ id: principalOwnerId(bob), kind: "user", name: "Bob", principal_id: bob })
    const v = ownerViews([team, adaOwner, bobOwner, goneOwner], directory)
    expect(v.get(adaOwner.id)).toMatchObject({ name: "Ada", inactive: false })
    expect(v.get(bobOwner.id)?.inactive).toBe(true)
    expect(v.get(goneOwner.id)).toMatchObject({ name: "Old group", inactive: true })
    expect(v.get("t1")).toMatchObject({ name: "Core", inactive: false })
  })

  it("assumes active until the directory loads", () => {
    expect(ownerViews([goneOwner], undefined).get(goneOwner.id)?.inactive).toBe(false)
  })
})
