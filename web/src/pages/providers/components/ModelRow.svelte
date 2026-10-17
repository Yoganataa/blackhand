<script lang="ts">
  // Ported from 9Router `dashboard/providers/[id]/ModelRow.js` (decolua/87a74e1).
  // Not ported here: CapacityBadges (context/capability caps) — the catalogue Blackhand fetches does
  // not expose per-model caps yet; and thinkingSuffix, which belongs with the thinking selector.
  let {
    model,
    fullModel,
    copied,
    onCopy,
    testStatus,
    isCustom = false,
    isFree = false,
    onDeleteAlias,
    onTest,
    isTesting = false,
    onDisable,
  }: {
    model: { id: string; name?: string };
    fullModel: string;
    copied?: string | null;
    onCopy: (text: string, id?: string) => void;
    testStatus?: "ok" | "error";
    isCustom?: boolean;
    isFree?: boolean;
    onDeleteAlias?: () => void;
    onTest?: () => void;
    isTesting?: boolean;
    onDisable?: () => void;
  } = $props();

  const borderColor = $derived(
    testStatus === "ok" ? "border-green-500/40" : testStatus === "error" ? "border-red-500/40" : "border-border",
  );
  const iconColor = $derived(testStatus === "ok" ? "#22c55e" : testStatus === "error" ? "#ef4444" : undefined);
  const copiedId = $derived(`model-${model.id}`);
</script>

<div class="group w-full sm:w-auto min-w-0 max-w-full rounded-lg border px-3 py-2 {borderColor} hover:bg-sidebar/50">
  <div class="flex min-w-0 items-start gap-2 sm:items-center">
    <span class="material-symbols-outlined shrink-0 text-base mt-0.5 sm:mt-0" style={iconColor ? `color:${iconColor}` : ""}>
      {testStatus === "ok" ? "check_circle" : testStatus === "error" ? "cancel" : "smart_toy"}
    </span>
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <code
        class="w-fit max-w-full break-all rounded bg-sidebar px-1.5 py-0.5 font-mono text-xs text-text-muted sm:max-w-[360px] sm:truncate"
        >{fullModel}</code
      >
      <span class="flex min-w-0 items-center text-[9px] gap-1 pl-1">
        {#if model.name}
          <span class="break-all sm:truncate text-[9px] italic text-text-muted/70">{model.name}</span>
        {/if}
        {#if isFree}
          <span class="text-[10px] font-bold text-green-500 bg-green-500/10 px-1.5 py-0.5 rounded">FREE</span>
        {/if}
      </span>
    </div>
    <div class="flex shrink-0 items-center gap-0.5 sm:gap-1">
      {#if onTest}
        <div class="relative shrink-0 group/btn">
          <button
            onclick={onTest}
            disabled={isTesting}
            class="rounded p-0.5 text-text-muted transition-opacity hover:bg-sidebar hover:text-primary {isTesting
              ? 'opacity-100'
              : 'opacity-100 sm:opacity-0 sm:group-hover:opacity-100'}"
          >
            <span class="material-symbols-outlined text-sm {isTesting ? 'animate-spin' : ''}"
              >{isTesting ? "progress_activity" : "science"}</span
            >
          </button>
          <span
            class="pointer-events-none absolute mt-1 top-5 left-1/2 -translate-x-1/2 text-[10px] text-text-muted whitespace-nowrap opacity-0 group-hover/btn:opacity-100 transition-opacity"
            >{isTesting ? "Testing..." : "Test"}</span
          >
        </div>
      {/if}
      <div class="relative shrink-0 group/btn">
        <button onclick={() => onCopy(fullModel, copiedId)} class="rounded p-0.5 text-text-muted hover:bg-sidebar hover:text-primary">
          <span class="material-symbols-outlined text-sm">{copied === copiedId ? "check" : "content_copy"}</span>
        </button>
        <span
          class="pointer-events-none absolute mt-1 top-5 left-1/2 -translate-x-1/2 text-[10px] text-text-muted whitespace-nowrap opacity-0 group-hover/btn:opacity-100 transition-opacity"
          >{copied === copiedId ? "Copied!" : "Copy"}</span
        >
      </div>
      {#if isCustom}
        <button
          onclick={onDeleteAlias}
          class="rounded p-0.5 text-text-muted opacity-100 transition-opacity hover:bg-red-500/10 hover:text-red-500 sm:opacity-0 sm:group-hover:opacity-100"
          title="Remove custom model"
        >
          <span class="material-symbols-outlined text-sm">close</span>
        </button>
      {:else if onDisable}
        <button
          onclick={onDisable}
          class="rounded p-0.5 text-text-muted opacity-100 transition-opacity hover:bg-red-500/10 hover:text-red-500 sm:opacity-0 sm:group-hover:opacity-100"
          title="Disable this model"
        >
          <span class="material-symbols-outlined text-sm">close</span>
        </button>
      {/if}
    </div>
  </div>
</div>
