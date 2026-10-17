<script lang="ts">
  // Ported 1:1 from 9Router `dashboard/endpoint/EndpointPageClient.js` (decolua/87a74e1, v0.5.104).
  // The Tunnel and Tailscale rows live on the same upstream card; they are not ported yet, so they
  // are absent rather than shown as dead controls. See docs/PORT_1TO1.md.
  import { onMount } from "svelte";
  import { createClipboard } from "../../lib/clipboard.svelte";
  import Button from "../../ui/Button.svelte";
  import Card from "../../ui/Card.svelte";
  import ConfirmModal from "../../ui/ConfirmModal.svelte";
  import Input from "../../ui/Input.svelte";
  import Modal from "../../ui/Modal.svelte";
  import Toggle from "../../ui/Toggle.svelte";
  import EndpointRow from "./components/EndpointRow.svelte";
  import KeyAccessControls from "./components/KeyAccessControls.svelte";
  import SecurityWarning from "./components/SecurityWarning.svelte";

  type KeyAccess = { restricted: boolean; allow: string[] };
  type APIKey = { id: string; key: string; name: string; isActive?: boolean; createdAt?: string; access?: KeyAccess };

  const clipboard = createClipboard();
  const copy = clipboard.copy;
  const copied = $derived(clipboard.copied);

  let keys = $state<APIKey[]>([]);
  let loading = $state(true);
  let showAddModal = $state(false);
  let newKeyName = $state("");
  let createdKey = $state<string | null>(null);
  let confirmState = $state<{ title: string; message: string; onConfirm: () => void } | null>(null);
  let requireApiKey = $state(false);
  let requireLogin = $state(true);
  let hasPassword = $state(true);
  let isRemoteHost = $state(false);
  let visibleKeys = $state(new Set<string>());
  let baseUrl = $state("/v1");
  let pickerKey = $state<APIKey | null>(null);
  let pickerModels = $state<string[]>([]);
  let pickerLoading = $state(false);

  // Security gate: block remote exposure while the dashboard uses the default password or login is off.
  const isLoginUnsafe = $derived(!requireLogin || !hasPassword);
  const unsafeReason = $derived(
    !requireLogin
      ? 'Enable "Require login" and set a custom password before activating the tunnel.'
      : "Change the default dashboard password before activating the tunnel.",
  );

  function maskKey(fullKey?: string) {
    if (!fullKey || fullKey.length <= 10) return fullKey || "";
    return fullKey.slice(0, 6) + "•".repeat(fullKey.length - 10) + fullKey.slice(-4);
  }

  function toggleKeyVisibility(id: string) {
    const next = new Set(visibleKeys);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    visibleKeys = next;
  }

  async function fetchKeys(): Promise<APIKey[]> {
    const res = await fetch("/api/keys");
    if (!res.ok) return [];
    const data = await res.json();
    return data.keys || [];
  }

  async function fetchData() {
    try {
      let existing = await fetchKeys();
      // Auto-provision a default key for first-time users so the endpoint works out of the box.
      if (existing.length === 0) {
        try {
          const createRes = await fetch("/api/keys", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: "Default Key" }),
          });
          if (createRes.ok) existing = await fetchKeys();
        } catch {
          // fall through to the empty render
        }
      }
      keys = existing;
    } finally {
      loading = false;
    }
  }

  async function loadSettings() {
    try {
      const res = await fetch("/api/settings", { cache: "no-store" });
      if (!res.ok) return;
      const data = await res.json();
      requireApiKey = data.requireApiKey || false;
      requireLogin = data.requireLogin !== false;
      hasPassword = data.hasPassword || false;
    } catch {
      // keep the defaults
    }
  }

  async function handleRequireApiKey(value: boolean) {
    try {
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ requireApiKey: value }),
      });
      if (res.ok) requireApiKey = value;
    } catch {
      // leave the toggle unchanged
    }
  }

  async function handleCreateKey() {
    if (!newKeyName.trim()) return;
    try {
      const res = await fetch("/api/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: newKeyName }),
      });
      const data = await res.json();
      if (res.ok) {
        createdKey = data.key;
        await fetchData();
        newKeyName = "";
        showAddModal = false;
      }
    } catch {
      // keep the modal open
    }
  }

  function handleDeleteKey(id: string) {
    confirmState = {
      title: "Delete API Key",
      message: "Delete this API key?",
      onConfirm: async () => {
        confirmState = null;
        try {
          const res = await fetch(`/api/keys/${encodeURIComponent(id)}`, { method: "DELETE" });
          if (res.ok) {
            keys = keys.filter((key) => key.id !== id);
            const next = new Set(visibleKeys);
            next.delete(id);
            visibleKeys = next;
          }
        } catch {
          // ignore
        }
      },
    };
  }

  async function handleToggleKey(id: string, isActive: boolean) {
    try {
      const res = await fetch(`/api/keys/${encodeURIComponent(id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ isActive }),
      });
      if (res.ok) keys = keys.map((key) => (key.id === id ? { ...key, isActive } : key));
    } catch {
      // ignore
    }
  }

  // Save a key's access (restricted flag + allow list).
  async function handleUpdateKeyAccess(id: string, access: KeyAccess) {
    try {
      const res = await fetch(`/api/keys/${encodeURIComponent(id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ access }),
      });
      const data = await res.json().catch(() => ({}));
      if (res.ok && data.key) keys = keys.map((key) => (key.id === id ? { ...key, access: data.key.access } : key));
    } catch {
      // ignore
    }
  }

  async function openPicker(key: APIKey) {
    pickerKey = key;
    pickerLoading = true;
    try {
      const res = await fetch("/v1/models", { cache: "no-store" });
      const data = await res.json().catch(() => ({}));
      pickerModels = (data.data || []).map((model: { id: string }) => model.id);
    } catch {
      pickerModels = [];
    } finally {
      pickerLoading = false;
    }
  }

  function addAllowed(value: string) {
    if (!pickerKey) return;
    const access = pickerKey.access || { restricted: true, allow: [] };
    if (access.allow.some((entry) => entry.toLowerCase() === value.toLowerCase())) return;
    void handleUpdateKeyAccess(pickerKey.id, { restricted: true, allow: [...access.allow, value] });
  }

  onMount(() => {
    if (typeof window !== "undefined") {
      baseUrl = `${window.location.origin}/v1`;
      isRemoteHost = !["localhost", "127.0.0.1", "::1"].includes(window.location.hostname);
    }
    void fetchData();
    void loadSettings();
  });
</script>

<div class="flex flex-col gap-8">
  <!-- Endpoint Card -->
  <Card>
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2">
      <span class="material-symbols-outlined text-primary">api</span>
      API Endpoint
    </h2>

    <div class="flex flex-col gap-2">
      <EndpointRow label="Local" url={baseUrl} copyId="local_url" copied={copied} onCopy={copy} />
    </div>

    {#if isLoginUnsafe}
      <div class="mt-4">
        <SecurityWarning message={unsafeReason} action={{ label: "Open settings", href: "/dashboard/profile" }} />
      </div>
    {/if}
  </Card>

  <!-- API Keys -->
  <Card id="require-api-key">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-lg font-semibold flex items-center gap-2">
        <span class="material-symbols-outlined text-primary">vpn_key</span>
        API Keys
      </h2>
      <Button icon="add" onclick={() => (showAddModal = true)}>Create Key</Button>
    </div>

    <div class="flex items-center justify-between pb-4 mb-4 border-b border-border">
      <div>
        <p class="font-medium">Require API key</p>
        <p class="text-sm text-text-muted">Requests without a valid key will be rejected</p>
      </div>
      <Toggle checked={requireApiKey} onChange={() => handleRequireApiKey(!requireApiKey)} />
    </div>

    {#if isRemoteHost && !requireApiKey}
      <div class="mb-4 -mt-2">
        <SecurityWarning message="Endpoint is exposed without an API key." />
      </div>
    {/if}

    {#if loading}
      <div class="py-12 text-center text-sm text-text-muted">Loading…</div>
    {:else if keys.length === 0}
      <div class="text-center py-12">
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-full bg-primary/10 text-primary mb-4">
          <span class="material-symbols-outlined text-[32px]">vpn_key</span>
        </div>
        <p class="text-text-main font-medium mb-1">No API keys yet</p>
        <p class="text-sm text-text-muted mb-4">Create your first API key to get started</p>
        <Button icon="add" onclick={() => (showAddModal = true)}>Create Key</Button>
      </div>
    {:else}
      <div class="flex flex-col">
        {#each keys as key (key.id)}
          <div
            class="group flex items-start justify-between gap-3 py-3 border-b border-black/[0.03] dark:border-white/[0.03] last:border-b-0 {key.isActive === false
              ? 'opacity-60'
              : ''}"
          >
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium">{key.name}</p>
              <div class="flex items-center gap-2 mt-1 min-w-0 flex-wrap">
                <code class="text-xs text-text-muted font-mono break-all min-w-0"
                  >{visibleKeys.has(key.id) ? key.key : maskKey(key.key)}</code
                >
                <div class="flex items-center gap-1 shrink-0">
                  <button
                    onclick={() => toggleKeyVisibility(key.id)}
                    class="p-1 hover:bg-black/5 dark:hover:bg-white/5 rounded text-text-muted hover:text-primary transition-all"
                    title={visibleKeys.has(key.id) ? "Hide key" : "Show key"}
                  >
                    <span class="material-symbols-outlined text-[14px]"
                      >{visibleKeys.has(key.id) ? "visibility_off" : "visibility"}</span
                    >
                  </button>
                  <button
                    onclick={() => copy(key.key, key.id)}
                    class="p-1 hover:bg-black/5 dark:hover:bg-white/5 rounded text-text-muted hover:text-primary transition-all"
                  >
                    <span class="material-symbols-outlined text-[14px]">{copied === key.id ? "check" : "content_copy"}</span>
                  </button>
                </div>
              </div>
              <p class="text-xs text-text-muted mt-1">
                Created {key.createdAt ? new Date(key.createdAt).toLocaleDateString() : "date unavailable"}
              </p>
              {#if key.isActive === false}
                <p class="text-xs text-orange-500 mt-1">Paused</p>
              {/if}
              <KeyAccessControls
                apiKey={key}
                onChange={(access) => handleUpdateKeyAccess(key.id, access)}
                onOpenPicker={() => openPicker(key)}
                onRequestRestrict={() =>
                  (confirmState = {
                    title: "Restrict API Key",
                    message: `Restrict API key "${key.name}"?\n\nIt will only be able to call the combos and models you add. Until you add one, it can call nothing.`,
                    onConfirm: async () => {
                      confirmState = null;
                      await handleUpdateKeyAccess(key.id, { restricted: true, allow: key.access?.allow || [] });
                    },
                  })}
              />
            </div>
            <div class="flex items-center gap-2 shrink-0 pt-0.5">
              <Toggle
                size="sm"
                checked={key.isActive ?? true}
                title={key.isActive ? "Pause key" : "Resume key"}
                onChange={(checked) => {
                  if (key.isActive && !checked) {
                    confirmState = {
                      title: "Pause API Key",
                      message: `Pause API key "${key.name}"?\n\nThis key will stop working immediately but can be resumed later.`,
                      onConfirm: async () => {
                        confirmState = null;
                        await handleToggleKey(key.id, checked);
                      },
                    };
                  } else {
                    void handleToggleKey(key.id, checked);
                  }
                }}
              />
              <button
                onclick={() => handleDeleteKey(key.id)}
                class="p-2 hover:bg-red-500/10 rounded text-red-500 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-all"
              >
                <span class="material-symbols-outlined text-[18px]">delete</span>
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </Card>
</div>

<!-- Add Key Modal -->
<Modal isOpen={showAddModal} title="Create API Key" onClose={() => { showAddModal = false; newKeyName = ""; }}>
  <div class="flex flex-col gap-4">
    <Input label="Key Name" bind:value={newKeyName} placeholder="Production Key" />
    <div class="flex gap-2">
      <Button onclick={handleCreateKey} fullWidth disabled={!newKeyName.trim()}>Create</Button>
      <Button
        onclick={() => {
          showAddModal = false;
          newKeyName = "";
        }}
        variant="ghost"
        fullWidth
      >
        Cancel
      </Button>
    </div>
  </div>
</Modal>

<!-- Created Key Modal -->
<Modal isOpen={!!createdKey} title="API Key Created" onClose={() => (createdKey = null)}>
  <div class="flex flex-col gap-4">
    <div class="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
      <p class="text-sm text-yellow-800 dark:text-yellow-200 mb-2 font-medium">Save this key now!</p>
      <p class="text-sm text-yellow-700 dark:text-yellow-300">
        This is the only time you will see this key. Store it securely.
      </p>
    </div>
    <div class="flex gap-2">
      <Input value={createdKey || ""} readOnly className="flex-1 font-mono text-sm" />
      <Button
        variant="secondary"
        icon={copied === "created_key" ? "check" : "content_copy"}
        onclick={() => copy(createdKey || "", "created_key")}
      >
        {copied === "created_key" ? "Copied!" : "Copy"}
      </Button>
    </div>
    <Button onclick={() => (createdKey = null)} fullWidth>Done</Button>
  </div>
</Modal>

<!-- Allowed models picker (interim: upstream uses the shared ModelSelectModal, which needs the Providers area) -->
<Modal isOpen={!!pickerKey} title={`Allowed for "${pickerKey?.name ?? ""}"`} onClose={() => (pickerKey = null)} size="lg">
  <div class="flex flex-col gap-2">
    {#if pickerLoading}
      <p class="text-sm text-text-muted flex items-center gap-2">
        <span class="material-symbols-outlined animate-spin text-sm">progress_activity</span>
        Loading models…
      </p>
    {:else if pickerModels.length === 0}
      <p class="text-sm text-text-muted">No models discovered. Check the provider configuration.</p>
    {:else}
      <p class="text-xs text-text-muted mb-1">Pick a model to allow for this key.</p>
      <div class="flex flex-wrap gap-1.5">
        {#each pickerModels as model (model)}
          <button
            onclick={() => addAllowed(model)}
            class="rounded-xl border border-primary/30 bg-primary/10 px-2 py-0.5 font-mono text-xs text-text-main hover:border-primary/60"
          >
            {model}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</Modal>

<ConfirmModal
  isOpen={!!confirmState}
  title={confirmState?.title}
  message={confirmState?.message}
  onClose={() => (confirmState = null)}
  onConfirm={() => confirmState?.onConfirm()}
/>
