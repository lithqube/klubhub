/**
 * useSidebar — desktop sidenav: full (220px) or an icon rail (56px).
 *
 * Persisted in a cookie rather than localStorage like the theme: the server can
 * read a cookie, so the first paint is already the right width. With
 * localStorage the page would render open and snap shut after hydration on
 * every load.
 */
export const useSidebar = () => {
  const collapsed = useCookie<boolean>('klubhub-nav-collapsed', {
    default: () => false,
    maxAge: 60 * 60 * 24 * 365,
    sameSite: 'lax',
  })

  function toggle() {
    collapsed.value = !collapsed.value
  }

  return { collapsed, toggle }
}
