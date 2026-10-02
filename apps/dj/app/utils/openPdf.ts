// Opens an exported PDF in a new tab (or downloads it, for same-origin and
// data: URLs, which honour `download`). An anchor click is used instead of
// window.open: it is not rewritten by popup blockers as often when it follows
// an await, and it works for the dev mock's data: URL, which browsers refuse
// to navigate to directly.
export function openPdf(url: string, filename = 'rider.pdf'): void {
  const a = document.createElement('a')
  a.href = url
  a.target = '_blank'
  a.rel = 'noopener noreferrer'
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
}
