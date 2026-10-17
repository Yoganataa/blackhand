<script lang="ts">
  // Ported from 9Router `dashboard/providers/[id]/CustomConfigCard.js` (decolua/87a74e1).
  // The server gate lives in internal/httpapi/provider_overrides.go; the checks here are UX only.
  import { onMount } from "svelte";
  import { toast } from "../../../lib/notifications.svelte";
  import Badge from "../../../ui/Badge.svelte";
  import Card from "../../../ui/Card.svelte";

  const BLOCKED_HEADERS = ["host", "content-length", "content-type", "connection", "transfer-encoding", "authorization", "cookie"];
  const HEADER_NAME_RE = /^[A-Za-z0-9-]+$/;

  let { providerId }: { providerId: string } = $props();

  type Row = { name: string; value: string };

  let expanded = $state(false);
  let rows = $state<Row[]>([{ name: "", value: "" }]);
  let builtin = $state<Record<string, string>>({});
  let hasOverride = $state(false);
  let saving = $state(false);

  const builtinCount = $derived(Object.keys(builtin).length);

  onMount(async () => {
    try {
      const res = await fetch(`/api/providers/${encodeURIComponent(providerId)}/overrides`, { cache: "no-store" });
      if (!res.ok) return;
      const data = await res.json();
      // Effective set = registry built-ins with user overrides layered on top.
      const builtinHeaders = data.builtinHeaders || {};
      const effective = { ...builtinHeaders, ...(data.headers || {}) };
      const headerRows = Object.entries(effective).map(([name, value]) => ({ name, value: String(value) }));
      builtin = builtinHeaders;
      rows = headerRows.length ? headerRows : [{ name: "", value: "" }];
      hasOverride = Object.keys(data.headers || {}).length > 0;
    } catch {
      // leave the card empty
    }
  });

  function setRow(index: number, field: keyof Row, value: string) {
    rows = rows.map((row, position) => (position === index ? { ...row, [field]: value } : row));
  }

  async function save() {
    // Diff-on-save: only rows differing from the registry default become overrides.
    const headers: Record<string, string> = {};
    for (const row of rows.filter((candidate) => candidate.name.trim())) {
      const name = row.name.trim();
      if (!HEADER_NAME_RE.test(name)) {
        toast.error(`Invalid header name: ${name}`);
        return;
      }
      if (BLOCKED_HEADERS.includes(name.toLowerCase())) {
        toast.error(`Header ${name} cannot be overridden`);
        return;
      }
      if (name in headers) {
        toast.error(`Duplicate header name: ${name}`);
        return;
      }
      if (row.value !== builtin[name]) headers[name] = row.value;
    }

    saving = true;
    try {
      const res = await fetch(`/api/providers/${encodeURIComponent(providerId)}/overrides`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ headers }),
      });
      if (!res.ok) {
        const error = await res.json().catch(() => ({}));
        toast.error(error.error || "Failed to save");
        return;
      }
      hasOverride = Object.keys(headers).length > 0;
      toast.success("Custom headers saved");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save");
    } finally {
      saving = false;
    }
  }

  function resetToBuiltin() {
    rows = Object.entries(builtin).map(([name, value]) => ({ name, value }));
  }
</script>

{#if builtinCount > 0 || hasOverride}
  <Card padding="xs">
    <button
      type="button"
      onclick={() => (expanded = !expanded)}
      class="flex w-full items-center justify-between text-left"
    >
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-primary text-[20px]">tune</span>
        <span class="text-sm font-semibold">Custom Headers</span>
        {#if hasOverride}<Badge variant="success" size="sm">Active</Badge>{/if}
      </div>
      <span class="material-symbols-outlined text-text-muted">{expanded ? "expand_less" : "expand_more"}</span>
    </button>

    {#if expanded}
      <div class="mt-3 border-t border-border pt-3">
        <div class="flex flex-col gap-2">
          {#each rows as row, index (index)}
            {@const overridden = row.name.trim() in builtin && row.value !== builtin[row.name.trim()]}
            <div class="flex items-center gap-2">
              <input
                value={row.name}
                oninput={(event) => setRow(index, "name", (event.currentTarget as HTMLInputElement).value)}
                placeholder="Header-Name"
                spellcheck="false"
                class="w-44 rounded-md border border-border bg-background px-2 py-1.5 text-sm focus:border-primary focus:outline-none"
              />
              <input
                value={row.value}
                oninput={(event) => setRow(index, "value", (event.currentTarget as HTMLInputElement).value)}
                placeholder="Value"
                spellcheck="false"
                title={overridden ? "Overridden" : row.name.trim() in builtin ? "Registry default" : ""}
                class="min-w-0 flex-1 rounded-md border bg-background px-2 py-1.5 text-sm focus:border-primary focus:outline-none {overridden
                  ? 'border-amber-400/60'
                  : 'border-border'}"
              />
              <button
                type="button"
                title="Remove header"
                onclick={() =>
                  (rows =
                    rows.length > 1
                      ? rows.filter((_, position) => position !== index)
                      : [{ name: "", value: "" }])}
                class="shrink-0 text-text-muted hover:text-red-500"
              >
                <span class="material-symbols-outlined text-[18px]">delete</span>
              </button>
            </div>
          {/each}
        </div>

        <div class="mt-3 flex items-center justify-between gap-2">
          <button
            type="button"
            onclick={() => (rows = [...rows, { name: "", value: "" }])}
            class="flex items-center gap-1 text-xs text-primary hover:underline"
          >
            <span class="material-symbols-outlined text-[16px]">add</span>
            Add header
          </button>
          <div class="flex gap-2">
            <button
              type="button"
              disabled={saving}
              onclick={resetToBuiltin}
              class="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-black/[0.03] dark:hover:bg-white/[0.03] disabled:opacity-50"
            >
              Reset
            </button>
            <button
              type="button"
              disabled={saving}
              onclick={save}
              class="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </div>
    {/if}
  </Card>
{/if}
