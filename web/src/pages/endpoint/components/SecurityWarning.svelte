<script lang="ts">
  // Ported from 9Router `dashboard/endpoint/components/SecurityWarning.js`.
  import { cn } from "../../../lib/cn";

  let {
    message,
    action,
  }: { message: string; action?: { label: string; href: string } } = $props();

  function handleActionClick(event: MouseEvent) {
    if (!action?.href.startsWith("#")) return;
    event.preventDefault();
    document.getElementById(action.href.slice(1))?.scrollIntoView({ behavior: "smooth" });
  }
</script>

<div
  class="flex items-center gap-2 px-3 py-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-700 dark:text-amber-400"
>
  <span class="material-symbols-outlined text-[16px] shrink-0 mt-0.5">warning</span>
  <p class="text-xs flex-1">{message}</p>
  {#if action}
    <a
      href={action.href}
      class={cn("text-xs font-medium underline shrink-0 hover:opacity-80")}
      onclick={handleActionClick}
    >
      {action.label}
    </a>
  {/if}
</div>
