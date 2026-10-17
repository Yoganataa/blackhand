<script lang="ts">
  // Ported from 9Router `dashboard/endpoint/components/EndpointRow.js`.
  import type { Snippet } from "svelte";
  import { cn } from "../../../lib/cn";
  import Input from "../../../ui/Input.svelte";

  let {
    label,
    url,
    copyId,
    copied,
    onCopy,
    badge,
    actions,
  }: {
    label: string;
    url: string;
    copyId: string;
    copied?: string | null;
    onCopy: (url: string, copyId: string) => void;
    badge?: string;
    actions?: Snippet;
  } = $props();
</script>

<div class="flex items-center gap-2">
  <span
    class={cn(
      "text-xs font-mono px-1.5 py-0.5 rounded shrink-0 min-w-[88px] text-center",
      badge === "CF" || badge === "TS" ? "bg-primary/10 text-primary" : "bg-surface-2 text-text-muted",
    )}>{label}</span
  >
  <Input value={url} readOnly className="flex-1 font-mono text-sm" />
  <button
    onclick={() => onCopy(url, copyId)}
    class="p-2 hover:bg-black/5 dark:hover:bg-white/5 rounded text-text-muted hover:text-primary transition-colors shrink-0"
  >
    <span class="material-symbols-outlined text-[18px]">{copied === copyId ? "check" : "content_copy"}</span>
  </button>
  {#if actions}{@render actions()}{/if}
</div>
