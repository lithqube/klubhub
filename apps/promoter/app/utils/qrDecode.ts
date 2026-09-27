/**
 * QR decoding for the door scanner (P2.3). BarcodeDetector is used where
 * the browser has it with 'qr_code' (Chrome on Android/macOS); elsewhere —
 * notably iOS Safari — frames go through jsQR, lazy-loaded so it stays out
 * of the main bundle and away from BarcodeDetector users.
 */
import type { Options as JsQROptions, QRCode } from 'jsqr'

export type JsQR = (data: Uint8ClampedArray, width: number, height: number, options?: JsQROptions) => QRCode | null

/** Frames are downscaled to at most this width before jsQR (speed vs. small codes). */
export const SCAN_WIDTH = 640

/** Decode interval for the jsQR path (~9 fps). */
export const SCAN_INTERVAL_MS = 110

/** Canvas size for a video frame: at most maxWidth wide, aspect kept, whole pixels. */
export function scaledSize(videoWidth: number, videoHeight: number, maxWidth = SCAN_WIDTH): { w: number, h: number } {
  if (!videoWidth || !videoHeight) return { w: 0, h: 0 }
  const scale = Math.min(1, maxWidth / videoWidth)
  return { w: Math.round(videoWidth * scale), h: Math.round(videoHeight * scale) }
}

/** The QR payload in an RGBA frame, or null. */
export function decodeFrame(jsqr: JsQR, rgba: Uint8ClampedArray, width: number, height: number): string | null {
  if (!width || !height || rgba.length < width * height * 4) return null
  const code = jsqr(rgba, width, height, { inversionAttempts: 'dontInvert' })
  return code?.data ? code.data : null
}

let jsqrModule: Promise<JsQR> | null = null

/** Load jsQR once (its own chunk under /_nuxt/, cached by the door service worker). */
export function loadJsQR(): Promise<JsQR> {
  jsqrModule ??= import('jsqr').then(m => (m.default ?? m) as unknown as JsQR)
  return jsqrModule
}

interface DetectorCtor {
  new (opts: { formats: string[] }): { detect(src: CanvasImageSource): Promise<{ rawValue: string }[]> }
  getSupportedFormats?: () => Promise<string[]>
}

/** A native QR detector when the browser supports one, else null. */
export async function nativeQrDetector(): Promise<InstanceType<DetectorCtor> | null> {
  const Ctor = (globalThis as unknown as { BarcodeDetector?: DetectorCtor }).BarcodeDetector
  if (!Ctor) return null
  try {
    const formats = (await Ctor.getSupportedFormats?.()) ?? ['qr_code']
    return formats.includes('qr_code') ? new Ctor({ formats: ['qr_code'] }) : null
  } catch {
    return null
  }
}
