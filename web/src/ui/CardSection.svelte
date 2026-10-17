<script lang="ts">
  // Ported from 9Router `src/shared/components/Card.js` → `Card.Section`.
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  let {
    children,
    className = "",
    class: klass = "",
    ...rest
  }: { children?: Snippet; className?: string; class?: string; [key: string]: unknown } = $props();
</script>

<div {...rest} class={cn("p-4 rounded-[10px] bg-bg border border-border-subtle", className, klass)}>
  {@render children?.()}
</div>
