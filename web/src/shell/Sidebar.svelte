<script lang="ts">
  // Ported 1:1 from 9Router `src/shared/components/Sidebar.js` (decolua/9router@87a74e1, v0.5.104).
  // Deferred to later phases, so they are absent rather than faked:
  //   - the npm update banner, ManualUpdatePanel and ConfirmModal (owned by the Setting area / version routes)
  //   - the Media Providers accordion, Skills and Translator (out of scope)
  //   - the 9Remote and 9English external links (upstream chrome pointing at third-party sites)
  import { APP_CONFIG } from "../lib/config";
  import { isActiveRoute } from "../lib/router.svelte";

  let { onClose = () => {} }: { onClose?: () => void } = $props();

  type NavItem = { href: string; label: string; icon: string };

  const navItems: NavItem[] = [
    { href: "/dashboard/endpoint", label: "Endpoint & Key", icon: "api" },
    { href: "/dashboard/providers", label: "Providers", icon: "dns" },
    { href: "/dashboard/usage", label: "Usage", icon: "bar_chart" },
    { href: "/dashboard/token-saver", label: "Token Saver", icon: "savings" },
  ];

  const systemItems: NavItem[] = [
    { href: "/dashboard/proxy-pools", label: "Proxy Pools", icon: "lan" },
  ];

  const debugItems: NavItem[] = [
    { href: "/dashboard/console-log", label: "Console Log", icon: "terminal" },
  ];

  const settingsItem: NavItem = { href: "/dashboard/profile", label: "Settings", icon: "settings" };

  const itemClass = (active: boolean) =>
    "flex items-center gap-3 px-3 py-1 rounded-lg transition-all group " +
    (active ? "bg-primary/10 text-primary" : "text-text-muted hover:bg-surface-2 hover:text-text-main");

  const iconClass = (active: boolean) =>
    "material-symbols-outlined text-[18px] " +
    (active ? "fill-1" : "group-hover:text-primary transition-colors");
</script>

{#snippet navLink(item: NavItem)}
  {@const active = isActiveRoute(item.href)}
  <a href={item.href} onclick={onClose} class={itemClass(active)}>
    <span class={iconClass(active)}>{item.icon}</span>
    <span class="text-[13px] font-medium">{item.label}</span>
  </a>
{/snippet}

<aside class="flex w-72 flex-col border-r border-border-subtle bg-vibrancy backdrop-blur-xl transition-colors duration-300 min-h-full">
  <!-- Traffic lights -->
  <div class="flex items-center gap-2 px-6 pt-5 pb-2">
    <div class="w-3 h-3 rounded-full bg-[#FF5F56]"></div>
    <div class="w-3 h-3 rounded-full bg-[#FFBD2E]"></div>
    <div class="w-3 h-3 rounded-full bg-[#27C93F]"></div>
  </div>

  <!-- Logo -->
  <div class="px-6 py-4 flex flex-col gap-2">
    <a href="/dashboard" class="flex items-center gap-3" onclick={onClose}>
      <div class="flex items-center justify-center size-9 rounded-[10px] bg-gradient-to-br from-brand-500 to-brand-700 shadow-[var(--shadow-warm)]">
        <span class="material-symbols-outlined text-white text-[20px]">hub</span>
      </div>
      <div class="flex flex-col">
        <h1 class="text-lg font-semibold tracking-tight text-text-main">{APP_CONFIG.name}</h1>
        <span class="text-xs text-text-muted">v{APP_CONFIG.version}</span>
      </div>
    </a>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 px-4 py-2 space-y-0.5 overflow-y-auto custom-scrollbar">
    {#each navItems as item (item.href)}
      {@render navLink(item)}
    {/each}

    <!-- System section -->
    <div class="pt-3 mt-2 space-y-0.5">
      <p class="px-4 text-xs font-semibold text-text-muted/60 uppercase tracking-wider mb-2">System</p>

      {#each systemItems as item (item.href)}
        {@render navLink(item)}
      {/each}

      {#each debugItems as item (item.href)}
        {@render navLink(item)}
      {/each}

      {@render navLink(settingsItem)}
    </div>
  </nav>
</aside>
