<script lang="ts">
  // Ported from the model section of 9Router `dashboard/providers/[id]/page.js` (decolua/87a74e1).
  // Model ids come from the live catalogue (/v1/models) because that is what this provider serves;
  // upstream additionally merges static registry models, a kilo free-model import and per-model caps.
  // Not ported here: the thinking selector, "Suggested free models", "Add Model" and the per-model
  // Test action — see docs/PORT_1TO1.md.
  import { onMount } from "svelte";
  import { createClipboard } from "../../../lib/clipboard.svelte";
  import Button from "../../../ui/Button.svelte";
  import Card from "../../../ui/Card.svelte";
  import ConfirmModal from "../../../ui/ConfirmModal.svelte";
  import ModelRow from "./ModelRow.svelte";

  let { providerAlias }: { providerAlias: string } = $props();

  type Model = { id: string; name?: string };

  const clipboard = createClipboard();
  const copy = clipboard.copy;
  const copied = $derived(clipboard.copied);

  let models = $state<Model[]>([]);
  let disabledIds = $state<string[]>([]);
  let loading = $state(true);
  let confirmState = $state<{ title: string; message: string; onConfirm: () => void } | null>(null);

  const disabledSet = $derived(new Set(disabledIds));
  const displayModels = $derived(models.filter((model) => !disabledSet.has(model.id)));
  const disabledModels = $derived(models.filter((model) => disabledSet.has(model.id)));

  async function load() {
    loading = true;
    try {
      const [modelsRes, disabledRes] = await Promise.all([
        // Dashboard-authenticated catalogue; /v1/models would need a client API key.
        fetch("/api/models", { cache: "no-store" }),
        fetch(`/api/models/disabled?providerAlias=${encodeURIComponent(providerAlias)}`, { cache: "no-store" }),
      ]);
      const modelsData = modelsRes.ok ? await modelsRes.json() : { models: [] };
      models = (modelsData.models || []).map((entry: { model: string; name?: string }) => ({
        id: String(entry.model),
        name: entry.name,
      }));
      const disabledData = disabledRes.ok ? await disabledRes.json() : { ids: [] };
      disabledIds = disabledData.ids || [];
    } catch {
      models = [];
      disabledIds = [];
    } finally {
      loading = false;
    }
  }

  async function disableModels(ids: string[]) {
    if (ids.length === 0) return;
    await fetch("/api/models/disabled", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ providerAlias, ids }),
    });
    await load();
  }

  async function enableModel(id: string) {
    await fetch(
      `/api/models/disabled?providerAlias=${encodeURIComponent(providerAlias)}&id=${encodeURIComponent(id)}`,
      { method: "DELETE" },
    );
    await load();
  }

  function handleDisableAll() {
    if (displayModels.length === 0) return;
    confirmState = {
      title: "Disable All Models",
      message: `Disable all ${displayModels.length} model(s)?`,
      onConfirm: async () => {
        confirmState = null;
        await disableModels(displayModels.map((model) => model.id));
      },
    };
  }

  onMount(() => {
    void load();
  });
</script>

<Card>
  <div class="flex items-center justify-between mb-4">
    <h2 class="text-lg font-semibold">Available Models</h2>
    <Button variant="secondary" size="sm" onclick={handleDisableAll} disabled={loading || displayModels.length === 0}>
      Disable All
    </Button>
  </div>

  {#if loading}
    <p class="text-sm text-text-muted flex items-center gap-2">
      <span class="material-symbols-outlined animate-spin text-sm">progress_activity</span>
      Loading models…
    </p>
  {:else if models.length === 0}
    <p class="text-sm text-text-muted">No models discovered from the catalogue.</p>
  {:else}
    <div class="flex flex-wrap gap-3">
      {#each displayModels as model (model.id)}
        <ModelRow
          {model}
          fullModel="{providerAlias}/{model.id}"
          {copied}
          onCopy={copy}
          onDisable={() => disableModels([model.id])}
        />
      {/each}
      {#each disabledModels as model (model.id)}
        <button
          onclick={() => enableModel(model.id)}
          class="flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-dashed border-black/10 dark:border-white/10 text-xs text-text-muted hover:text-primary hover:border-primary/40 hover:bg-primary/5 transition-colors"
          title="Restore model"
        >
          <span class="material-symbols-outlined text-[13px]">add</span>
          {model.id}
        </button>
      {/each}
    </div>
  {/if}
</Card>

<ConfirmModal
  isOpen={!!confirmState}
  title={confirmState?.title}
  message={confirmState?.message}
  onClose={() => (confirmState = null)}
  onConfirm={() => confirmState?.onConfirm()}
/>
