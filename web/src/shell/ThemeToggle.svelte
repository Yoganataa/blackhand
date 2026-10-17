<script lang="ts">
  // Ported from 9Router `src/shared/components/ThemeToggle.js` (decolua/9router@87a74e1).
  import { isDark, toggleTheme } from "../lib/theme.svelte";

  let { className = "", variant = "default" }: { className?: string; variant?: "default" | "card" } = $props();

  const variants = {
    default:
      "flex items-center justify-center size-10 rounded-full text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors",
    card: "flex items-center justify-center size-11 rounded-full bg-surface/60 hover:bg-surface border border-border backdrop-blur-md shadow-sm hover:shadow-[var(--shadow-warm)] text-text-muted hover:text-brand-500 transition-all group",
  };
</script>

<button
  onclick={toggleTheme}
  class="{variants[variant]} {className}"
  aria-label={isDark() ? "Switch to light mode" : "Switch to dark mode"}
  title={isDark() ? "Switch to light mode" : "Switch to dark mode"}
>
  <span
    class="material-symbols-outlined text-[22px] {variant === 'card' ? 'transition-transform duration-300 group-hover:rotate-12' : ''}"
  >
    {isDark() ? "light_mode" : "dark_mode"}
  </span>
</button>
