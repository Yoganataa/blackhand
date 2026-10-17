<script lang="ts">
  // Ported from 9Router `src/shared/components/Header.js` (decolua/9router@87a74e1).
  // Deferred to their own phases, so they are absent rather than dead controls:
  //   - provider detail breadcrumbs and the media-provider titles (Providers area follow-up)
  //   - OIDC/SAML identity chip (/api/auth/status, Setting area)
  //   - HeaderSearch, Donate button + DonateModal, HeaderLanguage, runtime i18n `translate()`
  import { route } from "../lib/router.svelte";
  import HeaderMenu from "./HeaderMenu.svelte";
  import ThemeToggle from "./ThemeToggle.svelte";

  let { onMenuClick = () => {}, onLogout }: { onMenuClick?: () => void; onLogout: () => void } = $props();

  type PageInfo = { title: string; description: string; icon: string };

  // Same order as upstream `getPageInfo`, restricted to the ported areas.
  function getPageInfo(pathname: string): PageInfo {
    if (pathname.includes("/providers") && !pathname.includes("/media-providers"))
      return { title: "Providers", description: "Manage your AI provider connections", icon: "dns" };
    if (pathname.includes("/usage"))
      return {
        title: "Usage & Analytics",
        description: "Monitor your API usage, token consumption, and request logs",
        icon: "bar_chart",
      };
    if (pathname.includes("/token-saver"))
      return { title: "Token Saver", description: "Compress prompts and outputs to save tokens", icon: "savings" };
    if (pathname.includes("/proxy-pools"))
      return { title: "Proxy Pools", description: "Manage your proxy pool configurations", icon: "lan" };
    if (pathname.includes("/endpoint"))
      return { title: "Endpoint", description: "API endpoint configuration", icon: "api" };
    if (pathname.includes("/profile"))
      return { title: "Settings", description: "Manage your preferences", icon: "settings" };
    if (pathname.includes("/console-log"))
      return { title: "Console Log", description: "Live server console output", icon: "monitor" };
    if (pathname === "/dashboard")
      return { title: "Endpoint", description: "API endpoint configuration", icon: "api" };
    return { title: "", description: "", icon: "" };
  }

  const pageInfo = $derived(getPageInfo(route.path));
</script>

<header class="shrink-0 flex items-center justify-between gap-3 px-4 lg:px-8 pt-3 pb-2 border-b border-border-subtle bg-surface/60 backdrop-blur-xl lg:bg-transparent lg:backdrop-blur-none z-20">
  <!-- Mobile menu button -->
  <div class="flex items-center gap-3 lg:hidden shrink-0">
    <button onclick={onMenuClick} class="text-text-main hover:text-primary transition-colors">
      <span class="material-symbols-outlined">menu</span>
    </button>
  </div>

  <!-- Page title -->
  <div class="flex flex-col min-w-0 flex-1">
    {#if pageInfo.title}
      <div>
        <div class="flex items-center gap-2">
          {#if pageInfo.icon}
            <span class="material-symbols-outlined text-primary text-xl lg:text-2xl">{pageInfo.icon}</span>
          {/if}
          <h1 class="text-base lg:text-2xl font-semibold tracking-tight truncate">{pageInfo.title}</h1>
        </div>
        {#if pageInfo.description}
          <p class="hidden lg:block text-sm text-text-muted truncate">{pageInfo.description}</p>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Right actions -->
  <div class="flex items-center gap-1 shrink-0">
    <ThemeToggle />
    <HeaderMenu {onLogout} />
  </div>
</header>
