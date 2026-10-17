<script lang="ts">
  // Ported from 9Router `dashboard/endpoint/components/KeyAccessControls.js`.
  // The "Add combo or model" picker is upstream's shared ModelSelectModal, which needs
  // /api/providers and /api/models/alias (Providers area). Until those exist the page opens a
  // models-only picker fed by /v1/models; see docs/PORT_1TO1.md.
  import Toggle from "../../../ui/Toggle.svelte";

  let {
    apiKey,
    onChange,
    onRequestRestrict,
    onOpenPicker,
  }: {
    apiKey: { id: string; name?: string; access?: { restricted: boolean; allow: string[] } };
    onChange: (access: { restricted: boolean; allow: string[] }) => void;
    onRequestRestrict: () => void;
    onOpenPicker: () => void;
  } = $props();

  const access = $derived(apiKey.access || { restricted: false, allow: [] });

  const remove = (value: string) => onChange({ restricted: true, allow: access.allow.filter((entry) => entry !== value) });
</script>

<div class="mt-2" data-testid="key-access">
  <Toggle
    size="sm"
    checked={access.restricted}
    label="Restrict access"
    onChange={(checked) => (checked ? onRequestRestrict() : onChange({ restricted: false, allow: access.allow }))}
  />
  {#if access.restricted}
    <div class="mt-2 flex flex-wrap items-center gap-1.5">
      {#if access.allow.length === 0}
        <span class="text-xs text-orange-500">No combos or models allowed - this key can call nothing.</span>
      {/if}
      {#each access.allow as entry (entry)}
        <span
          class="flex items-center gap-1 rounded-xl border border-primary/30 bg-primary/10 px-2 py-0.5 font-mono text-xs text-text-main"
        >
          {entry}
          <button
            onclick={() => remove(entry)}
            class="rounded text-text-muted hover:text-red-500"
            title="Remove {entry}"
            aria-label="Remove {entry}"
          >
            <span class="material-symbols-outlined leading-none" style="font-size: 12px">close</span>
          </button>
        </span>
      {/each}
      <button
        onclick={onOpenPicker}
        class="flex items-center gap-0.5 rounded-xl border border-dashed border-black/10 px-2 py-0.5 text-xs font-medium text-primary hover:border-primary/50 dark:border-white/10"
      >
        <span class="material-symbols-outlined leading-none" style="font-size: 14px">add</span>
        Add combo or model
      </button>
    </div>
  {/if}
</div>
