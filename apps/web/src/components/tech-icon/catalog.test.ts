import { describe, expect, it } from "vitest"

import { hasKnownIcon, knownTechnologies, technologySuggestions } from "./catalog"

describe("knownTechnologies", () => {
  it("only lists technologies that have an icon", () => {
    expect(knownTechnologies.filter((t) => !hasKnownIcon(t))).toEqual([])
  })
})

describe("technologySuggestions", () => {
  it("puts technologies used in the model first, most used first", () => {
    const out = technologySuggestions(["Redis", "Go", "Go", " Redis ", "Custom DB"])
    expect(out.slice(0, 3)).toEqual(["Go", "Redis", "Custom DB"])
  })

  it("does not repeat a known technology that differs only in case", () => {
    const out = technologySuggestions(["postgresql"])
    expect(out.filter((t) => t.toLowerCase() === "postgresql")).toEqual(["postgresql"])
  })

  it("ignores empty values", () => {
    expect(technologySuggestions(["", "  "])).toEqual([...knownTechnologies])
  })
})
