<script lang="ts">
  // Ported from 9Router `src/shared/components/Card.js` (decolua/9router@87a74e1).
  // `Card.Section` / `Card.Row` / `Card.ListItem` live in sibling components.
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  let {
    children,
    title,
    subtitle,
    icon,
    action,
    padding = "md",
    hover = false,
    elev = false,
    className = "",
    class: klass = "",
    ...rest
  }: {
    children?: Snippet;
    title?: string;
    subtitle?: string;
    icon?: string;
    action?: Snippet;
    padding?: "none" | "xs" | "sm" | "md" | "lg";
    hover?: boolean;
    elev?: boolean;
    className?: string;
    class?: string;
    [key: string]: unknown;
  } = $props();

  const paddings = { none: "", xs: "p-3", sm: "p-4", md: "p-6", lg: "p-8" };
</script>

<div
  {...rest}
  class={cn(
    "bg-surface border border-border-subtle",
    elev ? "rounded-[14px] shadow-[var(--shadow-elev)]" : "rounded-[14px] shadow-[var(--shadow-soft)]",
    hover && "hover:shadow-[var(--shadow-warm)] hover:border-brand-500/30 transition-all cursor-pointer",
    paddings[padding],
    className,
    klass,
  )}
>
  {#if title || action}
    <div class="flex items-center justify-between mb-4">
      <div class="flex items-center gap-3">
        {#if icon}
          <div class="p-2 rounded-[10px] bg-bg text-text-muted">
            <span class="material-symbols-outlined text-[20px]">{icon}</span>
          </div>
        {/if}
        <div>
          {#if title}<h3 class="text-text-main font-semibold">{title}</h3>{/if}
          {#if subtitle}<p class="text-sm text-text-muted">{subtitle}</p>{/if}
        </div>
      </div>
      {#if action}{@render action()}{/if}
    </div>
  {/if}
  {@render children?.()}
</div>
