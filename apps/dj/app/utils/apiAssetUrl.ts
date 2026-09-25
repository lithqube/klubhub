/**
 * URL for an <img src> that points at the API (post images, storage proxy).
 * Normally the path itself; the browser demo (app/demo) registers a resolver
 * that returns a local data URL instead, since it has no server.
 */
type AssetResolver = (path: string) => string | null

let resolver: AssetResolver | null = null

export function setApiAssetResolver(r: AssetResolver | null): void {
  resolver = r
}

export function apiAssetUrl(path: string): string {
  return resolver?.(path) ?? path
}
