<script lang="ts">
  // Ported from 9Router `src/shared/components/Toggle.js` (decolua/9router@87a74e1).
  import { cn } from "../lib/cn";

  let {
    checked = $bindable(false),
    onChange,
    label,
    description,
    disabled = false,
    size = "md",
    className = "",
    class: klass = "",
    title,
  }: {
    checked?: boolean;
    onChange?: (value: boolean) => void;
    label?: string;
    description?: string;
    disabled?: boolean;
    size?: "sm" | "md" | "lg";
    className?: string;
    class?: string;
    title?: string;
  } = $props();

  const sizes = {
    sm: { track: "w-8 h-4", thumb: "size-3", translate: "translate-x-4" },
    md: { track: "w-11 h-6", thumb: "size-5", translate: "translate-x-5" },
    lg: { track: "w-14 h-7", thumb: "size-6", translate: "translate-x-7" },
  };

  function handleClick() {
    if (disabled) return;
    const next = !checked;
    checked = next;
    onChange?.(next);
  }
</script>

<div class={cn("flex items-center gap-3", disabled && "opacity-50 cursor-not-allowed", className, klass)}>
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    {disabled}
    {title}
    onclick={handleClick}
    class={cn(
      "relative inline-flex shrink-0 cursor-pointer rounded-full",
      "transition-colors duration-200 ease-in-out",
      "focus:outline-none focus:ring-2 focus:ring-brand-500/30",
      checked ? "bg-brand-500" : "bg-surface-3",
      sizes[size].track,
      disabled && "cursor-not-allowed",
    )}
  >
    <span
      class={cn(
        "pointer-events-none inline-block rounded-full bg-white shadow-sm",
        "transform transition duration-200 ease-in-out mt-0.5",
        checked ? sizes[size].translate : "translate-x-0.5",
        sizes[size].thumb,
      )}
    ></span>
  </button>
  {#if label || description}
    <div class="flex flex-col">
      {#if label}<span class="text-sm font-medium text-text-main">{label}</span>{/if}
      {#if description}<span class="text-xs text-text-muted">{description}</span>{/if}
    </div>
  {/if}
</div>
