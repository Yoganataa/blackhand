<script lang="ts">
  // Ported 1:1 from 9Router `dashboard/profile/page.js` (decolua/87a74e1, v0.5.104).
  // Cards ported so far: Local Mode (theme + database location) and Security. Language, SSO
  // (OIDC/SAML), Routing Strategy, Network and Observability are not ported yet, so they are absent
  // rather than shown as dead controls — see docs/PORT_1TO1.md.
  import { onMount } from "svelte";
  import { cn } from "../../lib/cn";
  import { setTheme, theme } from "../../lib/theme.svelte";
  import Button from "../../ui/Button.svelte";
  import Card from "../../ui/Card.svelte";
  import Input from "../../ui/Input.svelte";
  import Toggle from "../../ui/Toggle.svelte";

  const THEME_OPTIONS = ["light", "dark", "system"] as const;

  let settings = $state<Record<string, any>>({});
  let loading = $state(true);
  let passwords = $state({ current: "", new: "", confirm: "" });
  let passStatus = $state({ type: "", message: "" });
  let passLoading = $state(false);

  async function loadSettings() {
    loading = true;
    try {
      const res = await fetch("/api/settings", { cache: "no-store" });
      if (res.ok) settings = await res.json();
    } finally {
      loading = false;
    }
  }

  async function updateRequireLogin(value: boolean) {
    const previous = settings.requireLogin;
    settings = { ...settings, requireLogin: value };
    try {
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ requireLogin: value }),
      });
      if (res.ok) settings = { ...settings, ...(await res.json()) };
      else settings = { ...settings, requireLogin: previous };
    } catch {
      settings = { ...settings, requireLogin: previous };
    }
  }

  async function handlePasswordChange(event: SubmitEvent) {
    event.preventDefault();
    passStatus = { type: "", message: "" };
    if (passwords.new !== passwords.confirm) {
      passStatus = { type: "error", message: "New passwords do not match" };
      return;
    }
    passLoading = true;
    try {
      const body: Record<string, string> = { newPassword: passwords.new };
      if (settings.hasPassword) body.currentPassword = passwords.current;
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        passStatus = { type: "error", message: data.error || "Could not update password" };
        return;
      }
      settings = { ...settings, ...data, hasPassword: true };
      passwords = { current: "", new: "", confirm: "" };
      passStatus = { type: "success", message: "Password updated" };
    } catch (error) {
      passStatus = { type: "error", message: error instanceof Error ? error.message : "Could not update password" };
    } finally {
      passLoading = false;
    }
  }

  onMount(() => {
    void loadSettings();
  });
</script>

<div class="flex flex-col gap-6">
  <!-- Local Mode -->
  <Card>
    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-4">
      <div class="flex items-center gap-3 sm:gap-4">
        <div class="size-10 sm:size-12 rounded-lg bg-green-500/10 text-green-500 flex items-center justify-center shrink-0">
          <span class="material-symbols-outlined text-xl sm:text-2xl">computer</span>
        </div>
        <div>
          <h2 class="text-lg sm:text-xl font-semibold">Local Mode</h2>
          <p class="text-sm text-text-muted">Running on your machine</p>
        </div>
      </div>
      <div class="inline-flex p-1 rounded-lg bg-black/5 dark:bg-white/5 w-full sm:w-auto">
        {#each THEME_OPTIONS as option (option)}
          <button
            type="button"
            onclick={() => setTheme(option)}
            class={cn(
              "flex items-center justify-center gap-1 sm:gap-1.5 px-2 sm:px-3 py-1.5 rounded-md font-medium transition-all flex-1 sm:flex-initial",
              theme.value === option ? "bg-white dark:bg-white/10 text-text-main shadow-sm" : "text-text-muted hover:text-text-main",
            )}
          >
            <span class="material-symbols-outlined text-[18px]">
              {option === "light" ? "light_mode" : option === "dark" ? "dark_mode" : "contrast"}
            </span>
            <span class="capitalize text-xs sm:text-sm">{option}</span>
          </button>
        {/each}
      </div>
    </div>
    <div class="flex flex-col gap-3 pt-4 border-t border-border">
      <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between p-3 rounded-lg bg-bg border border-border gap-2">
        <div>
          <p class="font-medium text-sm sm:text-base">Database Location</p>
          <p class="text-xs sm:text-sm text-text-muted font-mono break-all">{settings.databasePath || "…"}</p>
        </div>
      </div>
    </div>
  </Card>

  <!-- Security -->
  <Card>
    <div class="flex items-center gap-3 mb-4">
      <div class="p-2 rounded-lg bg-primary/10 text-primary shrink-0">
        <span class="material-symbols-outlined text-[20px]">shield</span>
      </div>
      <h3 class="text-base sm:text-lg font-semibold">Security</h3>
    </div>
    <div class="flex flex-col gap-4">
      <div class="flex items-start sm:items-center justify-between gap-4">
        <div class="flex-1 min-w-0">
          <p class="font-medium text-sm sm:text-base">Require login</p>
          <p class="text-xs sm:text-sm text-text-muted">When ON, dashboard requires password. When OFF, access without login.</p>
        </div>
        <Toggle
          checked={settings.requireLogin === true}
          onChange={() => updateRequireLogin(!settings.requireLogin)}
          disabled={loading}
        />
      </div>
      {#if settings.requireLogin === true}
        <form onsubmit={handlePasswordChange} class="flex flex-col gap-4 pt-4 border-t border-border/50">
          {#if settings.hasPassword}
            <div class="flex flex-col gap-2">
              <label class="text-xs sm:text-sm font-medium" for="current-password">Current Password</label>
              <Input id="current-password" type="password" placeholder="Enter current password" bind:value={passwords.current} required />
            </div>
          {/if}
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div class="flex flex-col gap-2">
              <label class="text-xs sm:text-sm font-medium" for="new-password">New Password</label>
              <Input id="new-password" type="password" placeholder="Enter new password" bind:value={passwords.new} required />
            </div>
            <div class="flex flex-col gap-2">
              <label class="text-xs sm:text-sm font-medium" for="confirm-password">Confirm New Password</label>
              <Input id="confirm-password" type="password" placeholder="Confirm new password" bind:value={passwords.confirm} required />
            </div>
          </div>

          {#if passStatus.message}
            <p class="text-xs sm:text-sm {passStatus.type === 'error' ? 'text-red-500' : 'text-green-500'}">{passStatus.message}</p>
          {/if}

          <div class="pt-2">
            <Button type="submit" variant="primary" loading={passLoading} className="w-full sm:w-auto">
              {settings.hasPassword ? "Update Password" : "Set Password"}
            </Button>
          </div>
        </form>
      {/if}
    </div>
  </Card>
</div>
