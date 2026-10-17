<script lang="ts">
  // Ported from 9Router `dashboard/providers/[id]/page.js` header (decolua/87a74e1, v0.5.104).
  // Cards in place: NoAuthProxyCard (no-auth providers). CustomConfigCard (custom headers),
  // ModelsCard (model list, enable/disable, thinking), connections and the rest of the detail page
  // are not ported yet, so they are absent rather than faked — see docs/PORT_1TO1.md.
  import { onMount } from "svelte";
  import { providerInfo } from "../../lib/providers";
  import ProviderIcon from "../../ui/ProviderIcon.svelte";
  import CustomConfigCard from "./components/CustomConfigCard.svelte";
  import AvailableModelsCard from "./components/AvailableModelsCard.svelte";
  import NoAuthProxyCard from "./components/NoAuthProxyCard.svelte";

  let { providerId }: { providerId: string } = $props();

  type Connection = { id: string; provider: string; isActive?: boolean };

  let connections = $state<Connection[]>([]);
  let loading = $state(true);
  let headerImgError = $state(false);

  const provider = $derived(providerInfo(providerId));

  onMount(async () => {
    try {
      const res = await fetch("/api/providers", { cache: "no-store" });
      const data = res.ok ? await res.json() : { connections: [] };
      connections = (data.connections || []).filter((connection: Connection) => connection.provider === providerId);
    } catch {
      connections = [];
    } finally {
      loading = false;
    }
  });
</script>

{#if loading}
  <div class="flex flex-col gap-8">
    {#each [0, 1] as skeleton (skeleton)}
      <div class="rounded-[14px] shadow-[var(--shadow-soft)] bg-surface border border-border-subtle p-6">
        <div class="h-4 w-40 rounded bg-surface-2 animate-pulse mb-3"></div>
        <div class="h-3 w-64 rounded bg-surface-2 animate-pulse"></div>
      </div>
    {/each}
  </div>
{:else if !provider}
  <div class="text-center py-20">
    <p class="text-text-muted">Provider not found</p>
    <a href="/dashboard/providers" class="text-primary mt-4 inline-block">Back to Providers</a>
  </div>
{:else}
  <div class="flex min-w-0 flex-col gap-6 px-1 sm:gap-8 sm:px-0">
    <!-- Header -->
    <div class="min-w-0">
      <a
        href="/dashboard/providers"
        class="inline-flex items-center gap-1 text-sm text-text-muted hover:text-primary transition-colors mb-4"
      >
        <span class="material-symbols-outlined text-lg">arrow_back</span>
        Back to Providers
      </a>
      <div class="flex min-w-0 items-center gap-3 sm:gap-4">
        <div
          class="flex size-12 shrink-0 items-center justify-center rounded-lg"
          style="background-color: {provider.color}15"
        >
          {#if headerImgError}
            <span class="text-sm font-bold" style="color: {provider.color}">{provider.textIcon}</span>
          {:else}
            <ProviderIcon
              providerId={provider.id}
              alt={provider.name}
              size={48}
              className="object-contain rounded-lg size-12"
              fallbackText={provider.textIcon}
              fallbackColor={provider.color}
            />
          {/if}
        </div>
        <div class="min-w-0">
          <div class="flex items-center gap-3 flex-wrap">
            <h1 class="truncate text-2xl font-semibold tracking-tight sm:text-3xl">{provider.name}</h1>
          </div>
          <p class="text-text-muted">{connections.length} connection{connections.length === 1 ? "" : "s"}</p>
        </div>
      </div>
    </div>

    {#if provider.noAuth}
      <NoAuthProxyCard providerId={provider.id} />
    {/if}

    <CustomConfigCard providerId={provider.id} />

    <AvailableModelsCard providerAlias={provider.alias} />
  </div>
{/if}
