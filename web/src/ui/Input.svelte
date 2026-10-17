<script module lang="ts">
  let counter = 0;
</script>

<script lang="ts">
  // Ported from 9Router `src/shared/components/Input.js` (decolua/87a74e1).
  // Upstream controls the value with onChange; Svelte call sites use bind:value.
  // The label is wired to the input with a generated id so the a11y check stays clean.
  import { cn } from "../lib/cn";

  const inputId = `input-${++counter}`;

  let {
    label,
    type = "text",
    placeholder,
    value = $bindable(""),
    error,
    hint,
    icon,
    disabled = false,
    required = false,
    className = "",
    inputClassName = "",
    class: klass = "",
    ...rest
  }: {
    label?: string;
    type?: string;
    placeholder?: string;
    value?: string;
    error?: string;
    hint?: string;
    icon?: string;
    disabled?: boolean;
    required?: boolean;
    className?: string;
    inputClassName?: string;
    class?: string;
    [key: string]: unknown;
  } = $props();
</script>

<div class={cn("flex flex-col gap-1.5", className, klass)}>
  {#if label}
    <label for={inputId} class="text-sm font-medium text-text-main">
      {label}
      {#if required}<span class="text-red-500 ml-1">*</span>{/if}
    </label>
  {/if}
  <div class="relative">
    {#if icon}
      <div class="absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none text-text-muted">
        <span class="material-symbols-outlined text-[20px]">{icon}</span>
      </div>
    {/if}
    <input
      {...rest}
      id={inputId}
      {type}
      {placeholder}
      bind:value
      {disabled}
      class={cn(
        "w-full py-2.5 px-3 text-sm text-text-main bg-surface-2 rounded-[10px]",
        "border border-transparent placeholder-text-muted/70",
        "focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/40",
        "transition-all duration-150 ease-out disabled:opacity-50 disabled:cursor-not-allowed",
        "text-[16px] sm:text-sm",
        icon && "pl-10",
        error && "ring-1 ring-red-500 focus:ring-2 focus:ring-red-500/40 border-red-500/40",
        inputClassName,
      )}
    />
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
