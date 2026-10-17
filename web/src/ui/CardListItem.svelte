<script lang="ts">
  // Ported from 9Router `src/shared/components/Card.js` → `Card.ListItem`.
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  let {
    children,
    actions,
    className = "",
    class: klass = "",
    ...rest
  }: { children?: Snippet; actions?: Snippet; className?: string; class?: string; [key: string]: unknown } = $props();
</script>

<div
  {...rest}
  class={cn(
    "group flex items-center justify-between p-3 -mx-3 px-3 border-b border-border-subtle last:border-b-0 hover:bg-surface-2/50 transition-colors",
    className,
    klass,
  )}
>
  <div class="flex-1 min-w-0">{@render children?.()}</div>
  {#if actions}
    <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
      {@render actions()}
    </div>
  {/if}
</div>
