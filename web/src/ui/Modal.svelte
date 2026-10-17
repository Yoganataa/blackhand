<script lang="ts">
  // Ported from 9Router `src/shared/components/Modal.js` (decolua/9router@87a74e1).
  import type { Snippet } from "svelte";
  import { cn } from "../lib/cn";
  import Tooltip from "./Tooltip.svelte";

  let {
    isOpen = false,
    onClose,
    title,
    children,
    footer,
    size = "md",
    closeOnOverlay = true,
    showTrafficLights = true,
    className = "",
  }: {
    isOpen?: boolean;
    onClose?: () => void;
    title?: string;
    children?: Snippet;
    footer?: Snippet;
    size?: "sm" | "md" | "lg" | "xl" | "full";
    closeOnOverlay?: boolean;
    showTrafficLights?: boolean;
    className?: string;
  } = $props();

  const sizes = {
    sm: "max-w-sm",
    md: "max-w-md",
    lg: "max-w-lg",
    xl: "max-w-xl",
    full: "max-w-4xl",
  };

  $effect(() => {
    if (typeof document === "undefined") return;
    document.body.style.overflow = isOpen ? "hidden" : "";
    return () => {
      document.body.style.overflow = "";
    };
  });

  $effect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isOpen) onClose?.();
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  });
</script>

{#if isOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    {#if closeOnOverlay}
      <button
        type="button"
        class="absolute inset-0 bg-black/50 backdrop-blur-[2px] fade-in"
        aria-label="Close dialog"
        onclick={onClose}
      ></button>
    {:else}
      <div class="absolute inset-0 bg-black/50 backdrop-blur-[2px] fade-in" role="presentation"></div>
    {/if}

    <div
      class={cn(
        "relative w-full bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-elev)] fade-in",
        sizes[size],
        className,
      )}
    >
      {#if title || showTrafficLights}
        <div class="flex items-center justify-between p-2 border-b border-border-subtle">
          <div class="flex items-center">
            {#if showTrafficLights}
              <div class="hidden md:flex items-center gap-2 mr-4 ml-2">
                <Tooltip text="Close" position="top" color="#FF5F56">
                  <button
                    onclick={onClose}
                    aria-label="Close"
                    title="Close"
                    class="w-4 h-4 rounded-full bg-[#FF5F56] hover:brightness-90 transition-all cursor-pointer flex items-center justify-center group/dot"
                  >
                    <span
                      class="text-[9px] font-bold text-white opacity-0 group-hover/dot:opacity-100 transition-opacity leading-none"
                    >×</span>
                  </button>
                </Tooltip>
                <div class="w-4 h-4 rounded-full bg-[#3a3a3a]/20 dark:bg-white/15 cursor-not-allowed"></div>
                <div class="w-4 h-4 rounded-full bg-[#3a3a3a]/20 dark:bg-white/15 cursor-not-allowed"></div>
              </div>
            {/if}
            {#if title}<h2 class="text-lg font-semibold text-text-main">{title}</h2>{/if}
          </div>
          <button
            onclick={onClose}
            aria-label="Close"
            class="md:hidden p-1.5 rounded-[10px] text-text-muted hover:bg-surface-2 hover:text-text-main transition-colors"
          >
            <span class="material-symbols-outlined text-[20px]">close</span>
          </button>
        </div>
      {/if}

      <div class="p-6 max-h-[calc(85vh-100px)] overflow-y-auto custom-scrollbar">
        {@render children?.()}
      </div>

      {#if footer}
        <div class="flex items-center justify-end gap-3 p-6 border-t border-border-subtle">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
