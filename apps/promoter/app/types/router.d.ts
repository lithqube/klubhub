declare module '#app' {
  interface PageMeta {
    /** Reachable without a session; rendered without the app shell. */
    public?: boolean
  }
}

export {}
