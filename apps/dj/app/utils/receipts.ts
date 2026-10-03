// Client-side checks for receipt files. They only spare a doomed request:
// the server reads the type from the file's bytes and stays authoritative.

export const RECEIPT_MAX_BYTES = 15 * 1024 * 1024
export const RECEIPT_MAX_FILES = 10
export const RECEIPT_ACCEPT = 'image/jpeg,image/png,image/webp,application/pdf'
export const RECEIPT_HINT = 'JPEG, PNG, WebP or PDF, up to 15 MB each, 10 per entry'

const ALLOWED_MIME = new Set(['image/jpeg', 'image/png', 'image/webp', 'application/pdf'])
const ALLOWED_EXT = new Set(['jpg', 'jpeg', 'png', 'webp', 'pdf'])
// Some browsers and drag sources report no type (or a generic one) for a file
// that is fine: the extension decides then.
const UNKNOWN_MIME = new Set(['', 'application/octet-stream'])

function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot === -1 ? '' : name.slice(dot + 1).toLowerCase()
}

/** Human size: "340 KB", "1.2 MB". */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  // Rounded up, so a file just over the limit never reads as exactly the limit.
  return `${Math.ceil((bytes / (1024 * 1024)) * 10) / 10} MB`
}

export function isImageReceipt(mimeOrName: { type?: string; name?: string; mime_type?: string }): boolean {
  const mime = (mimeOrName.mime_type ?? mimeOrName.type ?? '').toLowerCase()
  if (mime) return mime.startsWith('image/')
  return ['jpg', 'jpeg', 'png', 'webp'].includes(extensionOf(mimeOrName.name ?? ''))
}

/** Why a picked file cannot be a receipt, or null when it can. */
export function checkReceiptFile(file: { name: string; type: string; size: number }): string | null {
  const mime = file.type.toLowerCase()
  const typeOk = ALLOWED_MIME.has(mime) || (UNKNOWN_MIME.has(mime) && ALLOWED_EXT.has(extensionOf(file.name)))
  if (!typeOk) return `${file.name}: use a JPEG, PNG, WebP or PDF file.`
  if (file.size === 0) return `${file.name} is empty.`
  if (file.size > RECEIPT_MAX_BYTES) {
    return `${file.name} is ${formatBytes(file.size)}. Receipts can be up to ${RECEIPT_MAX_BYTES / (1024 * 1024)} MB.`
  }
  return null
}
