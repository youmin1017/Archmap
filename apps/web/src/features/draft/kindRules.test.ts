import { describe, expect, it } from "vitest"

import { allowedParentKinds, parseTags, requiresParent } from "./kindRules"

describe("kindRules", () => {
  it("mirrors the hierarchy the lint rules enforce", () => {
    expect(allowedParentKinds.component).toEqual(["container"])
    expect(allowedParentKinds.container).toEqual(["system"])
    expect(allowedParentKinds.node).toEqual(["node"])
    expect(allowedParentKinds.person).toEqual([])
    expect(requiresParent("container")).toBe(true)
    expect(requiresParent("node")).toBe(false)
  })

  it("parses tags", () => {
    expect(parseTags("a, b ,,c, a")).toEqual(["a", "b", "c"])
    expect(parseTags("")).toEqual([])
    expect(parseTags("  ")).toEqual([])
  })
})
