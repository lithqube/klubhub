import { encode } from 'uqr'
import { describe, expect, it } from 'vitest'
import { decodeFrame, loadJsQR, scaledSize } from '../qrDecode'

/** Render a QR matrix into an RGBA buffer: `px` pixels per module, 4-module quiet zone. */
function rgbaFor(text: string, px = 4) {
  const { data, size } = encode(text)
  const quiet = 4
  const w = (size + quiet * 2) * px
  const buf = new Uint8ClampedArray(w * w * 4).fill(255)
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      if (!data[y]![x]) continue
      for (let dy = 0; dy < px; dy++) {
        for (let dx = 0; dx < px; dx++) {
          const i = (((y + quiet) * px + dy) * w + (x + quiet) * px + dx) * 4
          buf[i] = buf[i + 1] = buf[i + 2] = 0
        }
      }
    }
  }
  return { buf, w }
}

describe('qrDecode', () => {
  it('decodes a ticket secret from an RGBA frame with jsQR', async () => {
    const jsqr = await loadJsQR()
    const { buf, w } = rgbaFor('DICE-0001')
    expect(decodeFrame(jsqr, buf, w, w)).toBe('DICE-0001')
  })

  it('returns null for a blank frame or a short buffer', async () => {
    const jsqr = await loadJsQR()
    expect(decodeFrame(jsqr, new Uint8ClampedArray(64 * 64 * 4).fill(255), 64, 64)).toBeNull()
    expect(decodeFrame(jsqr, new Uint8ClampedArray(10), 64, 64)).toBeNull()
  })

  it('downscales frames to at most 640 px wide', () => {
    expect(scaledSize(1920, 1080)).toEqual({ w: 640, h: 360 })
    expect(scaledSize(480, 640)).toEqual({ w: 480, h: 640 })
    expect(scaledSize(0, 0)).toEqual({ w: 0, h: 0 })
  })
})
