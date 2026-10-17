<script lang="ts">
  // Ported from 9Router `src/shared/components/NoAuthProxyCard.js` (decolua/87a74e1).
  // Per-provider override lives in settings.providerStrategies[providerId] = { proxyPoolId, rotateStrategy }.
  import { onMount } from "svelte";
  import Badge from "../../../ui/Badge.svelte";
  import Card from "../../../ui/Card.svelte";
  import Select from "../../../ui/Select.svelte";

  const NONE_PROXY_POOL_VALUE = "__none__";
  const STRATEGIES = [
    { value: "none", label: "None (single pool)" },
    { value: "round-robin", label: "Round-robin" },
    { value: "random", label: "Random" },
  ];

  let { providerId }: { providerId: string } = $props();

  type ProxyPool = { id: string; name: string; isActive?: boolean };

  let proxyPools = $state<ProxyPool[]>([]);
  let proxyPoolId = $state(NONE_PROXY_POOL_VALUE);
  let rotateStrategy = $state("none");
  let saving = $state(false);
  let savedFlash = $state(false);

  const canRotate = $derived(proxyPools.length >= 2);
  const isRotation = $derived(rotateStrategy !== "none");

  const poolOptions = $derived([
    { value: NONE_PROXY_POOL_VALUE, label: "None (direct)" },
    ...proxyPools.map((pool) => ({ value: pool.id, label: pool.name })),
  ]);

  const rotationHint = $derived(
    !canRotate
      ? "Need at least 2 active proxy pools for rotation."
      : isRotation
        ? rotateStrategy === "round-robin"
          ? `Rotating through all ${proxyPools.length} active pools in order. State is in-memory (resets on restart).`
          : `Picking a random pool from ${proxyPools.length} active pools each request.`
        : "Uses the selected pool above. Set to Round-robin or Random to rotate across all active pools.",
  );

  async function load() {
    try {
      const [poolRes, settingsRes] = await Promise.all([
        fetch("/api/proxy-pools?includeUsage=true", { cache: "no-store" }),
        fetch("/api/settings", { cache: "no-store" }),
      ]);
      // Upstream filters with ?isActive=true on the proxy-pool route; Blackhand filters here.
      const poolData = poolRes.ok ? await poolRes.json() : { proxyPools: [] };
      proxyPools = (poolData.proxyPools || []).filter((pool: ProxyPool) => pool.isActive !== false);
      const settingsData = settingsRes.ok ? await settingsRes.json() : {};
      const override = (settingsData.providerStrategies || {})[providerId] || {};
      proxyPoolId = override.proxyPoolId || NONE_PROXY_POOL_VALUE;
      rotateStrategy = override.rotateStrategy || "none";
    } catch {
      // keep the defaults
    }
  }

  async function save(poolId: string, strategy: string) {
    saving = true;
    try {
      const res = await fetch("/api/settings", { cache: "no-store" });
      const data = res.ok ? await res.json() : {};
      const current = data.providerStrategies || {};
      const override: Record<string, string> = { ...(current[providerId] || {}) };
      if (poolId === NONE_PROXY_POOL_VALUE) delete override.proxyPoolId;
      else override.proxyPoolId = poolId;
      if (strategy === "none") delete override.rotateStrategy;
      else override.rotateStrategy = strategy;

      const updated: Record<string, unknown> = { ...current };
      if (Object.keys(override).length === 0) delete updated[providerId];
      else updated[providerId] = override;

      await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ providerStrategies: updated }),
      });
      savedFlash = true;
      setTimeout(() => (savedFlash = false), 1500);
    } catch {
      // leave the selection as typed
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    void load();
  });
</script>

<Card>
  <div class="flex items-center gap-3 mb-4">
    <div class="inline-flex items-center justify-center w-10 h-10 rounded-full bg-green-500/10 text-green-500">
      <span class="material-symbols-outlined text-[20px]">lock_open</span>
    </div>
    <div class="flex-1">
      <p class="text-sm font-medium">No authentication required</p>
      <p class="text-xs text-text-muted">
        This provider is ready to use. Optionally route requests through a proxy pool to bypass IP-based limits.
      </p>
    </div>
    {#if savedFlash}<Badge variant="success" size="sm">Saved</Badge>{/if}
  </div>

  <Select
    label="Proxy Pool"
    bind:value={proxyPoolId}
    onChange={(value) => {
      proxyPoolId = value;
      void save(value, rotateStrategy);
    }}
    disabled={saving || isRotation}
    options={poolOptions}
    hint={isRotation ? "Pool selector is ignored when rotation is active - all active pools are used." : undefined}
  />

  <div class="flex flex-col gap-2 mt-4">
    <label class="text-sm font-medium text-text-main" for="rotation-strategy">Rotation Strategy</label>
    <select
      id="rotation-strategy"
      bind:value={rotateStrategy}
      disabled={saving}
      onchange={() => void save(proxyPoolId, rotateStrategy)}
      class="py-2 px-3 text-sm text-text-main bg-white dark:bg-white/5 border border-black/10 dark:border-white/10 rounded-md focus:ring-1 focus:ring-primary/30 focus:border-primary/50 focus:outline-none transition-all disabled:opacity-50"
    >
      {#each STRATEGIES as strategy (strategy.value)}
        <option value={strategy.value} disabled={strategy.value !== "none" && !canRotate}>{strategy.label}</option>
      {/each}
    </select>
    <p class="text-xs text-text-muted">{rotationHint}</p>
  </div>
</Card>
