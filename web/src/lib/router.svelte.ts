// Minimal path router with real URLs, so the Go server can serve the SPA for /dashboard/*.
// Upstream relies on the Next.js App Router; this keeps the same pathnames.
export const route = $state<{ path: string }>({
  path: typeof window === "undefined" ? "/" : window.location.pathname,
});

export function navigate(href: string, replace = false): void {
  if (href === route.path) return;
  if (replace) window.history.replaceState(null, "", href);
  else window.history.pushState(null, "", href);
  route.path = href;
  window.scrollTo({ top: 0 });
}

// Upstream treats /dashboard (the index redirect) as the Endpoint & Key entry.
export function isActiveRoute(href: string): boolean {
  if (href === "/dashboard/endpoint") {
    return route.path === "/dashboard" || route.path.startsWith("/dashboard/endpoint");
  }
  return route.path.startsWith(href);
}

if (typeof window !== "undefined") {
  window.addEventListener("popstate", () => {
    route.path = window.location.pathname;
  });

  // Delegate internal dashboard links to the router, like next/link does.
  window.addEventListener("click", (event: MouseEvent) => {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const target = event.target;
    const anchor = target instanceof Element ? target.closest("a[href]") : null;
    if (!(anchor instanceof HTMLAnchorElement) || anchor.target || anchor.hasAttribute("download")) return;
    const href = anchor.getAttribute("href");
    if (!href) return;
    const url = new URL(href, window.location.origin);
    if (url.origin !== window.location.origin) return;
    if (!url.pathname.startsWith("/dashboard") && url.pathname !== "/") return;
    event.preventDefault();
    navigate(url.pathname + url.search);
  });
}
