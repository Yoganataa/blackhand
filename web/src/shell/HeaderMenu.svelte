<script lang="ts">
  // Ported from 9Router `src/shared/components/HeaderMenu.js` (decolua/9router@87a74e1).
  // Deferred, so they are absent rather than dead buttons:
  //   - "Change Log" (ChangelogModal + /api/version/changelog)
  //   - "Shutdown" (/api/version/shutdown) — lands with the Setting area's version routes
  import { isDark, toggleTheme } from "../lib/theme.svelte";

  let { onLogout }: { onLogout: () => void } = $props();

  let isOpen = $state(false);
  let menuEl: HTMLDivElement | undefined = $state();

  $effect(() => {
    if (!isOpen) return;
    const onDown = (event: MouseEvent) => {
      const target = event.target;
      if (menuEl && target instanceof Node && !menuEl.contains(target)) isOpen = false;
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  });

  const menuItemClass = (danger: boolean) =>
    "flex items-center gap-3 w-full px-4 py-2.5 text-sm transition-colors " +
    (danger ? "text-red-500 hover:bg-red-500/10" : "text-text-main hover:bg-black/5 dark:hover:bg-white/5");
</script>

{#snippet menuItem(icon: string, label: string, onclick: () => void, danger: boolean)}
  <button {onclick} class={menuItemClass(danger)}>
    <span class="material-symbols-outlined text-[20px] {danger ? '' : 'text-text-muted'}">{icon}</span>
    <span class="flex-1 text-left">{label}</span>
  </button>
{/snippet}

<div class="relative" bind:this={menuEl}>
  <button
    onclick={() => (isOpen = !isOpen)}
    class="flex items-center justify-center p-2 rounded-lg text-text-muted hover:text-text-main hover:bg-black/5 dark:hover:bg-white/5 transition-all"
    title="Menu"
  >
    <span class="material-symbols-outlined">grid_view</span>
  </button>

  {#if isOpen}
    <div class="absolute right-0 top-full mt-2 w-60 bg-surface border border-black/10 dark:border-white/10 rounded-xl shadow-2xl z-50 animate-in fade-in zoom-in-95 duration-150 overflow-hidden py-1">
      {@render menuItem(isDark() ? "light_mode" : "dark_mode", "Theme", () => { toggleTheme(); isOpen = false; }, false)}
      {@render menuItem("logout", "Logout", () => { isOpen = false; onLogout(); }, true)}
    </div>
  {/if}
</div>
