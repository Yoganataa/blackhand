<script lang="ts">
  // Ported from 9Router `src/shared/components/Card.js` → `Card.Row`.
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  let {
    children,
    className = "",
    class: klass = "",
    ...rest
  }: { children?: Snippet; className?: string; class?: string; [key: string]: unknown } = $props();
</script>

<div
  {...rest}
  class={cn(
    "p-3 -mx-3 px-3 transition-colors border-b border-border-subtle last:border-b-0 hover:bg-surface-2/50",
    className,
    klass,
  )}
>
  {@render children?.()}
</div>
