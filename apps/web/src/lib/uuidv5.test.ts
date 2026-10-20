import { describe, expect, it } from "vitest"

import { principalOwnerId, uuidv5 } from "./uuidv5"

describe("uuidv5", () => {
  it("matches RFC 4122 (dns namespace, python.org)", () => {
    const dns = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
    expect(uuidv5(dns, new TextEncoder().encode("python.org"))).toBe("886313e1-3b8a-5372-9b90-0c9aee199e5d")
  })

  it("derives the same owner id as the API (model.PrincipalOwnerID)", () => {
    expect(principalOwnerId("00000000-0000-7000-8000-0000000000d1")).toBe("a45b7d5a-b7f6-5f1f-842b-af5a59336dc5")
  })
})
