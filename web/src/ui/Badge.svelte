<script lang="ts">
  // Ported from 9Router `src/shared/components/Badge.js` (decolua/87a74e1).
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  type Variant = "default" | "primary" | "success" | "warning" | "error" | "info";

  let {
    children,
    variant = "default",
    size = "md",
    dot = false,
    icon,
    className = "",
  }: {
    children?: Snippet;
    variant?: Variant;
    size?: "sm" | "md" | "lg";
    dot?: boolean;
    icon?: string;
    className?: string;
  } = $props();

  const variants: Record<Variant, string> = {
    default: "bg-surface-2 text-text-muted",
    primary: "bg-brand-500/10 text-brand-600 dark:text-brand-300",
    success: "bg-green-500/10 text-green-600 dark:text-green-400",
    warning: "bg-yellow-500/10 text-yellow-600 dark:text-yellow-400",
    error: "bg-red-500/10 text-red-600 dark:text-red-400",
    info: "bg-blue-500/10 text-blue-600 dark:text-blue-400",
  };

  const sizes = {
    sm: "px-2 py-0.5 text-[10px]",
    md: "px-2.5 py-1 text-xs",
    lg: "px-3 py-1.5 text-sm",
  };

  const dotColors: Record<Variant, string> = {
    default: "bg-gray-500",
    primary: "bg-brand-500",
    success: "bg-green-500",
    warning: "bg-yellow-500",
    error: "bg-red-500",
    info: "bg-blue-500",
  };
</script>

<span class={cn("inline-flex items-center gap-1.5 rounded-full font-semibold", variants[variant], sizes[size], className)}>
  {#if dot}<span class={cn("size-1.5 rounded-full", dotColors[variant])}></span>{/if}
  {#if icon}<span class="material-symbols-outlined text-[14px]">{icon}</span>{/if}
  {@render children?.()}
</span>
