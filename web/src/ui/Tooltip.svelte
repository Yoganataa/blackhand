<script lang="ts">
  // Ported from 9Router `src/shared/components/Modal.js` (decolua/9router@87a74e1).
  // ponytail: upstream renders the bubble through a React portal; Svelte has none built in, so the
  // bubble is positioned fixed but stays in the DOM tree. Add a manual body portal if a clipping
  // ancestor ever breaks it.
  import type { Snippet } from "svelte";

  let {
    text,
    children,
    position = "top",
    color,
  }: {
    text: string;
    children?: Snippet;
    position?: "top" | "bottom" | "left" | "right";
    color?: string;
  } = $props();

  let anchor: HTMLSpanElement | undefined = $state();
  let style = $state<Record<string, string> | null>(null);

  function show() {
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const midX = rect.left + rect.width / 2;
    const midY = rect.top + rect.height / 2;
    let pos: Record<string, string>;
    if (position === "bottom") pos = { top: `${rect.bottom + 6}px`, left: `${midX}px`, transform: "translateX(-50%)" };
    else if (position === "left") pos = { right: `${window.innerWidth - rect.left + 6}px`, top: `${midY}px`, transform: "translateY(-50%)" };
    else if (position === "right") pos = { left: `${rect.right + 6}px`, top: `${midY}px`, transform: "translateY(-50%)" };
    else pos = { bottom: `${window.innerHeight - rect.top + 6}px`, left: `${midX}px`, transform: "translateX(-50%)" };
    style = { position: "fixed", ...pos };
  }

  const styleString = $derived(style ? Object.entries(style).map(([k, v]) => `${k}:${v}`).join(";") : "");
</script>

<span
  bind:this={anchor}
  class="relative inline-flex"
  role="presentation"
  onmouseenter={show}
  onmouseleave={() => (style = null)}
  onfocusin={show}
  onfocusout={() => (style = null)}
>
  {@render children?.()}
  {#if style}
    <div
      class="pointer-events-none z-50 w-max max-w-56 rounded px-2 py-1 text-[11px] leading-snug text-white whitespace-normal {color
        ? ''
        : 'bg-gray-900'}"
      style="{styleString};{color ? `background-color:${color}` : ''}"
    >
      {text}
    </div>
  {/if}
</span>
