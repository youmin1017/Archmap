import { useEffect, useRef, useState } from "react"

import type { Box } from "./layout"

export const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3)

const lerp = (a: number, b: number, t: number) => a + (b - a) * t

/** Interpolates every box present in both maps; boxes only in `to` appear at their target. */
export function lerpBoxes(from: ReadonlyMap<string, Box>, to: ReadonlyMap<string, Box>, t: number): Map<string, Box> {
  const out = new Map<string, Box>()
  for (const [id, target] of to) {
    const start = from.get(id)
    out.set(
      id,
      start
        ? {
            x: lerp(start.x, target.x, t),
            y: lerp(start.y, target.y, t),
            width: lerp(start.width, target.width, t),
            height: lerp(start.height, target.height, t),
          }
        : target
    )
  }
  return out
}

/**
 * Smoothly follows `target`: when the target layout changes (expand / collapse),
 * node boxes are tweened from where they currently are, so nodes and the edges
 * attached to them move together. With `animate` false the change is immediate
 * (prefers-reduced-motion).
 */
export function useAnimatedBoxes(target: ReadonlyMap<string, Box>, animate: boolean, durationMs = 280) {
  const [boxes, setBoxes] = useState<ReadonlyMap<string, Box>>(target)
  const current = useRef<ReadonlyMap<string, Box>>(target)

  useEffect(() => {
    if (!animate || current.current.size === 0) {
      current.current = target
      setBoxes(target)
      return
    }
    const from = current.current
    const start = performance.now()
    let frame = 0
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / durationMs)
      const next = t >= 1 ? target : lerpBoxes(from, target, easeOutCubic(t))
      current.current = next
      setBoxes(next)
      if (t < 1) frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [target, animate, durationMs])

  return boxes
}
