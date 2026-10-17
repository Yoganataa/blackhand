<script lang="ts">
  // Ported from 9Router `src/shared/components/Button.js` (decolua/9router@87a74e1).
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";

  type Variant = "primary" | "secondary" | "outline" | "ghost" | "danger" | "success";
  type Size = "xs" | "sm" | "md" | "lg";

  let {
    children,
    variant = "primary",
    size = "md",
    icon,
    iconRight,
    disabled = false,
    loading = false,
    fullWidth = false,
    className = "",
    class: klass = "",
    ...rest
  }: {
    children?: Snippet;
    variant?: Variant;
    size?: Size;
    icon?: string;
    iconRight?: string;
    disabled?: boolean;
    loading?: boolean;
    fullWidth?: boolean;
    className?: string;
    class?: string;
    [key: string]: unknown;
  } = $props();

  const variants: Record<Variant, string> = {
    primary: "bg-brand-500 hover:bg-brand-600 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted",
    secondary: "bg-surface-2 hover:bg-surface-3 text-text-main border border-border disabled:opacity-50",
    outline: "border border-border text-text-main hover:bg-surface-2 hover:border-brand-500/40",
    ghost: "text-text-muted hover:bg-surface-2 hover:text-text-main",
    danger: "bg-red-500 hover:bg-red-600 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted",
    success: "bg-green-600 hover:bg-green-700 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted",
  };

  const sizes: Record<Size, string> = {
    xs: "h-6 px-2.5 text-[11px] rounded-[6px] gap-1.5",
    sm: "h-7 px-3 text-xs rounded-[8px]",
    md: "h-9 px-4 text-sm rounded-[10px]",
    lg: "h-11 px-6 text-sm rounded-[10px]",
  };

  const iconSize = $derived(size === "xs" ? "text-[14px]" : "text-[18px]");
</script>

<button
  {...rest}
  class={cn(
    "inline-flex items-center justify-center gap-2 font-semibold transition-all duration-150 ease-out cursor-pointer",
    "active:scale-[0.97] disabled:opacity-50 disabled:cursor-not-allowed disabled:active:scale-100",
    variants[variant],
    sizes[size],
    fullWidth && "w-full",
    className,
    klass,
  )}
  disabled={disabled || loading}
>
  {#if loading}
    <span class={cn("material-symbols-outlined animate-spin", iconSize)}>progress_activity</span>
  {:else if icon}
    <span class={cn("material-symbols-outlined", iconSize)}>{icon}</span>
  {/if}
  {@render children?.()}
  {#if iconRight && !loading}
    <span class={cn("material-symbols-outlined", iconSize)}>{iconRight}</span>
  {/if}
</button>
