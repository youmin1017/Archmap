// RFC 4122 name-based UUID (version 5, SHA-1), synchronous. The API derives some ids
// this way (user / group owners, ADR-069); the web computes the same id so it can add
// an owner and assign it in one batch.

function sha1(bytes: Uint8Array): Uint8Array {
  const ml = bytes.length * 8
  const withPad = new Uint8Array(((bytes.length + 9 + 63) >> 6) << 6)
  withPad.set(bytes)
  withPad[bytes.length] = 0x80
  const view = new DataView(withPad.buffer)
  view.setUint32(withPad.length - 4, ml >>> 0)
  view.setUint32(withPad.length - 8, Math.floor(ml / 2 ** 32))
  let h0 = 0x67452301
  let h1 = 0xefcdab89
  let h2 = 0x98badcfe
  let h3 = 0x10325476
  let h4 = 0xc3d2e1f0
  const w = new Uint32Array(80)
  const rotl = (x: number, n: number) => (x << n) | (x >>> (32 - n))
  for (let off = 0; off < withPad.length; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4)
    for (let i = 16; i < 80; i++) w[i] = rotl(w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16], 1)
    let [a, b, c, d, e] = [h0, h1, h2, h3, h4]
    for (let i = 0; i < 80; i++) {
      const [f, k] =
        i < 20
          ? [(b & c) | (~b & d), 0x5a827999]
          : i < 40
            ? [b ^ c ^ d, 0x6ed9eba1]
            : i < 60
              ? [(b & c) | (b & d) | (c & d), 0x8f1bbcdc]
              : [b ^ c ^ d, 0xca62c1d6]
      const t = (rotl(a, 5) + f + e + k + w[i]) >>> 0
      e = d
      d = c
      c = rotl(b, 30) >>> 0
      b = a
      a = t
    }
    h0 = (h0 + a) >>> 0
    h1 = (h1 + b) >>> 0
    h2 = (h2 + c) >>> 0
    h3 = (h3 + d) >>> 0
    h4 = (h4 + e) >>> 0
  }
  const out = new Uint8Array(20)
  const ov = new DataView(out.buffer)
  ;[h0, h1, h2, h3, h4].forEach((h, i) => ov.setUint32(i * 4, h))
  return out
}

const parse = (uuid: string) =>
  Uint8Array.from(
    uuid
      .replaceAll("-", "")
      .match(/../g)!
      .map((b) => parseInt(b, 16))
  )

const format = (b: Uint8Array) => {
  const hex = [...b].map((x) => x.toString(16).padStart(2, "0")).join("")
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`
}

/** uuidv5(namespace, name) with the name given as bytes (Go's uuid.NewSHA1). */
export function uuidv5(namespace: string, name: Uint8Array): string {
  const ns = parse(namespace)
  const buf = new Uint8Array(ns.length + name.length)
  buf.set(ns)
  buf.set(name, ns.length)
  const hash = sha1(buf).slice(0, 16)
  hash[6] = (hash[6] & 0x0f) | 0x50
  hash[8] = (hash[8] & 0x3f) | 0x80
  return format(hash)
}

/** Namespace of user / group owner ids (model.PrincipalOwnerID in the API). */
const PRINCIPAL_OWNER_NS = "0d6b8f3a-9e2c-5b41-8a7d-3f5e6c1b2a90"

/** The id of the owner that stands for a user or group. */
export const principalOwnerId = (principalId: string) => uuidv5(PRINCIPAL_OWNER_NS, parse(principalId))
