<script lang="ts">
  // Ported from 9Router `src/shared/components/Select.js` (decolua/87a74e1).
  import { cn } from "../lib/cn";

  const selectId = `select-${Math.random().toString(36).slice(2, 9)}`;

  let {
    label,
    options = [],
    value = $bindable(""),
    onChange,
    placeholder = "Select an option",
    error,
    hint,
    disabled = false,
    className = "",
    selectClassName = "",
    class: klass = "",
    ...rest
  }: {
    label?: string;
    options?: { value: string; label: string }[];
    value?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    error?: string;
    hint?: string;
    disabled?: boolean;
    className?: string;
    selectClassName?: string;
    class?: string;
    [key: string]: unknown;
  } = $props();
</script>

<div class={cn("flex flex-col gap-1.5", className, klass)}>
  {#if label}
    <label for={selectId} class="text-sm font-medium text-text-main">{label}</label>
  {/if}
  <div class="relative">
    <select
      {...rest}
      id={selectId}
      bind:value
      {disabled}
      onchange={() => onChange?.(value)}
      class={cn(
        "w-full py-2.5 px-3 pr-10 text-sm text-text-main",
        "bg-surface-2 border border-transparent rounded-[10px] appearance-none",
        "focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/40",
        "transition-all duration-150 disabled:opacity-50 disabled:cursor-not-allowed",
        "text-[16px] sm:text-sm",
        error && "ring-1 ring-red-500 focus:ring-2 focus:ring-red-500/40 border-red-500/40",
        selectClassName,
      )}
    >
      <option value="" disabled>{placeholder}</option>
      {#each options as option (option.value)}
        <option value={option.value}>{option.label}</option>
      {/each}
    </select>
    <div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-text-muted">
      <span class="material-symbols-outlined text-[20px]">expand_more</span>
    </div>
  </div>
  {#if error}
    <p class="text-xs text-red-500 flex items-center gap-1">
      <span class="material-symbols-outlined text-[14px]">error</span>
      {error}
    </p>
  {:else if hint}
    <p class="text-xs text-text-muted">{hint}</p>
  {/if}
</div>
