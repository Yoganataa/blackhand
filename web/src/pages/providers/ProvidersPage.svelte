<script lang="ts">
  // Ported from 9Router `dashboard/providers/page.js` Free Tier section (decolua/87a74e1).
  // Only the Free Tier group is ported, because OpenCode Free is the only provider in scope.
  // The batch "Test All" action belongs with the connection test endpoints, not ported yet.
  import { onMount } from "svelte";
  import { FREE_TIER_PROVIDERS } from "../../lib/providers";
  import Badge from "../../ui/Badge.svelte";
  import Card from "../../ui/Card.svelte";
  import ProviderIcon from "../../ui/ProviderIcon.svelte";

  type Connection = { id: string; provider: string; isActive?: boolean };

  let connections = $state<Connection[]>([]);

  onMount(async () => {
    try {
      const res = await fetch("/api/providers", { cache: "no-store" });
      const data = res.ok ? await res.json() : { connections: [] };
      connections = data.connections || [];
    } catch {
      connections = [];
    }
  });

  const connectionsFor = (providerId: string) => connections.filter((connection) => connection.provider === providerId);
  const allDisabled = (providerId: string) => {
    const list = connectionsFor(providerId);
    return list.length > 0 && list.every((connection) => connection.isActive === false);
  };
</script>

<div class="flex flex-col gap-8">
  {#if FREE_TIER_PROVIDERS.length > 0}
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="text-lg sm:text-xl font-semibold flex items-center gap-2 leading-tight">Free Tier Providers</h2>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
        {#each FREE_TIER_PROVIDERS as provider (provider.id)}
          <a href="/dashboard/providers/{provider.id}" class="group min-w-0">
            <Card
              padding="xs"
              className="h-full hover:bg-black/[0.01] dark:hover:bg-white/[0.01] transition-colors cursor-pointer"
            >
              <div class="flex min-w-0 items-center justify-between gap-3">
                <div class="flex min-w-0 items-center gap-3">
                  <div
                    class="size-8 shrink-0 rounded-lg flex items-center justify-center"
                    style="background-color: {provider.color}15"
                  >
                    <ProviderIcon
                      providerId={provider.id}
                      alt={provider.name}
                      size={30}
                      className="object-contain rounded-lg max-w-[32px] max-h-[32px]"
                      fallbackText={provider.textIcon}
                      fallbackColor={provider.color}
                    />
                  </div>
                  <div class="min-w-0">
                    <h3 class="truncate font-semibold">{provider.name}</h3>
                    <div class="flex min-w-0 items-center gap-1.5 text-xs flex-wrap">
                      {#if allDisabled(provider.id)}
                        <Badge variant="default" size="sm">
                          <span class="flex items-center gap-1">
                            <span class="material-symbols-outlined text-[12px]">pause_circle</span>
                            Disabled
                          </span>
                        </Badge>
                      {:else if provider.noAuth}
                        <Badge variant="success" size="sm" dot>Ready</Badge>
                      {/if}
                    </div>
                  </div>
                </div>
              </div>
            </Card>
          </a>
        {/each}
      </div>
    </div>
  {/if}
</div>
