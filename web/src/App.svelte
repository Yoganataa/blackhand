<script lang="ts">
  import { onMount } from "svelte";
  import DashboardLayout from "./shell/DashboardLayout.svelte";
  import EndpointPage from "./pages/endpoint/EndpointPage.svelte";
  import ProfilePage from "./pages/profile/ProfilePage.svelte";
  import { navigate, route } from "./lib/router.svelte";

  type Section = "Endpoint & Key" | "Providers" | "Usage" | "Token Saver" | "Console Log" | "Settings" | "Proxypools";
  type ProxyPool = { id: string; name: string; proxyUrl: string; noProxy?: string; type: string; isActive: boolean; strictProxy?: boolean; testStatus?: string; lastError?: string; boundConnectionCount?: number };
  type Stats = { totalRequests?: number; totalPromptTokens?: number; totalCompletionTokens?: number; totalCachedTokens?: number; totalCost?: number; byProvider?: Record<string, any>; recentRequests?: any[]; activeRequests?: any[]; last10Minutes?: any[] };
  type ChartPoint = { label: string; tokens: number; cost: number; requests: number };

  // Upstream paths per ported area (docs/PORT_1TO1.md). The shell sidebar owns the navigation;
  // this only maps the current URL to the area whose data must load.
  const sectionPaths: Record<Section, string> = {
    "Endpoint & Key": "/dashboard/endpoint",
    "Providers": "/dashboard/providers",
    "Usage": "/dashboard/usage",
    "Token Saver": "/dashboard/token-saver",
    "Console Log": "/dashboard/console-log",
    "Settings": "/dashboard/profile",
    "Proxypools": "/dashboard/proxy-pools"
  };
  function sectionFromPath(pathname: string): Section {
    if (pathname === "/" || pathname === "/dashboard") return "Endpoint & Key";
    const matched = (Object.entries(sectionPaths) as [Section, string][]).find(([, path]) => pathname.startsWith(path));
    return matched?.[0] || "Endpoint & Key";
  }
  function pageTitle(section: Section): string {
    if (section === "Endpoint & Key") return "Endpoint";
    if (section === "Proxypools") return "Proxy Pools";
    return section;
  }

  const active = $derived(sectionFromPath(route.path));
  let authenticated = $state(false);
  let loginRequired = $state(true);
  let hasPassword = $state(false);
  let password = $state("");
  let authError = $state("");
  let actionError = $state("");
  let successMessage = $state("");
  let busy = $state(false);
  let probeKey = $state("");
  let models = $state<any[]>([]);
  let modelProbeMessage = $state("");
  let settings = $state<Record<string, any>>({});
  let stats = $state<Stats>({});
  let chart = $state<ChartPoint[]>([]);
  let period = $state("7d");
  let logs = $state<string[]>([]);
  let consoleStream: EventSource | null = null;
  let pools = $state<ProxyPool[]>([]);
  let poolName = $state("");
  let poolUrl = $state("");
  let poolType = $state("http");
  let poolNoProxy = $state("");
  let poolStrict = $state(false);

  const num = (value: unknown) => Number(value || 0).toLocaleString();
  const percentWidth = (value: number, max: number) => max > 0 ? Math.max(value > 0 ? 2 : 0, value / max * 100) : 0;

  async function request(path: string, init: RequestInit = {}) {
    const headers = new Headers(init.headers || {});
    if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    const response = await fetch(path, { ...init, headers, cache: "no-store" });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const nested = payload && payload.error && payload.error.message;
      throw new Error(nested || (payload && payload.error) || ("HTTP " + response.status));
    }
    return payload;
  }

  async function checkSession() {
    try {
      const session = await request("/api/auth/session");
      authenticated = Boolean(session.authenticated);
      loginRequired = Boolean(session.loginRequired);
      hasPassword = Boolean(session.hasPassword);
      if (authenticated) await refreshSection(active);
    } catch (error) {
      authenticated = false;
      authError = error instanceof Error ? error.message : "Could not check dashboard session";
    }
  }
  async function login(event: SubmitEvent) {
    event.preventDefault();
    authError = "";
    busy = true;
    try {
      const result = await request("/api/auth/login", { method: "POST", body: JSON.stringify({ password }) });
      authenticated = true;
      hasPassword = Boolean(result.mustChangePassword) ? false : hasPassword;
      password = "";
      await refreshSection(active);
    } catch (error) { authError = error instanceof Error ? error.message : "Login failed"; }
    finally { busy = false; }
  }
  async function logout() {
    try { await request("/api/auth/logout", { method: "POST" }); } catch { /* session may have expired */ }
    stopConsoleStream();
    authenticated = false;
    loginRequired = true;
    settings = {};
    models = [];
    pools = [];
  }
  async function refreshSection(section: Section) {
    actionError = "";
    try {
      // Endpoint & Key and Setting own their data: those pages load what they need themselves.
      if (section === "Providers") await loadModels();
      if (section === "Usage") await loadUsage();
      if (section === "Token Saver") await loadSettings();
      if (section === "Console Log") await openConsoleStream();
      if (section === "Proxypools") await loadPools();
    } catch (error) { actionError = error instanceof Error ? error.message : "Could not load this section"; }
  }
  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text); successMessage = "Copied to clipboard"; }
    catch { actionError = "Clipboard access was denied by the browser"; }
  }
  async function loadModels() {
    modelProbeMessage = "Checking OpenCode model catalogue…";
    const response = await fetch("/v1/models", { headers: probeKey ? { Authorization: "Bearer " + probeKey } : {}, cache: "no-store" });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      modelProbeMessage = (result && result.error && result.error.message) || result.error || ("Model discovery failed: HTTP " + response.status);
      models = [];
      return;
    }
    models = result.data || [];
    modelProbeMessage = models.length + " model(s) discovered";
  }
  async function loadSettings() { settings = await request("/api/settings"); }
  async function saveSetting(key: string, value: any) {
    actionError = "";
    successMessage = "";
    const oldValue = settings[key];
    settings = { ...settings, [key]: value };
    try {
      const result = await request("/api/settings", { method: "PATCH", body: JSON.stringify({ [key]: value }) });
      settings = { ...settings, ...result };
      successMessage = "Setting saved";
    } catch (error) {
      settings = { ...settings, [key]: oldValue };
      actionError = error instanceof Error ? error.message : "Could not save setting";
    }
  }
  async function loadUsage() {
    const values = await Promise.all([
      request("/api/usage/stats?period=" + encodeURIComponent(period)),
      request("/api/usage/chart?period=" + encodeURIComponent(period))
    ]);
    stats = values[0];
    chart = values[1] || [];
  }
  async function changePeriod(value: string) { period = value; await loadUsage(); }
  async function loadLogs() {
    const result = await request("/api/translator/console-logs");
    logs = result.logs || [];
  }
  async function openConsoleStream() {
    await loadLogs();
    stopConsoleStream();
    consoleStream = new EventSource("/api/translator/console-logs/stream");
    consoleStream.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.type === "clear") logs = [];
        else if (payload.type === "line" && payload.line) logs = [...logs, payload.line].slice(-500);
        else if (payload.type === "init" && payload.logs) logs = payload.logs;
      } catch { /* ignore malformed event */ }
    };
  }
  function stopConsoleStream() { consoleStream?.close(); consoleStream = null; }
  async function clearLogs() {
    try { await request("/api/translator/console-logs", { method: "DELETE" }); logs = []; successMessage = "Console buffer cleared"; }
    catch (error) { actionError = error instanceof Error ? error.message : "Could not clear logs"; }
  }
  async function loadPools() {
    const result = await request("/api/proxy-pools?includeUsage=true");
    pools = result.proxyPools || [];
  }
  async function createPool(event: SubmitEvent) {
    event.preventDefault();
    actionError = "";
    try {
      await request("/api/proxy-pools", { method: "POST", body: JSON.stringify({ name: poolName, proxyUrl: poolUrl, noProxy: poolNoProxy, type: poolType, strictProxy: poolStrict, isActive: true }) });
      poolName = ""; poolUrl = ""; poolNoProxy = ""; poolStrict = false;
      await loadPools(); successMessage = "Proxy pool created";
    } catch (error) { actionError = error instanceof Error ? error.message : "Could not create proxy pool"; }
  }
  async function togglePool(pool: ProxyPool) {
    try { await request("/api/proxy-pools/" + encodeURIComponent(pool.id), { method: "PUT", body: JSON.stringify({ isActive: !pool.isActive }) }); await loadPools(); }
    catch (error) { actionError = error instanceof Error ? error.message : "Could not update proxy pool"; }
  }
  async function testPool(pool: ProxyPool) {
    try {
      const result = await request("/api/proxy-pools/" + encodeURIComponent(pool.id) + "/test", { method: "POST" });
      await loadPools();
      successMessage = result.ok ? ("Proxy test passed in " + result.elapsedMs + " ms") : result.error || "Proxy test failed";
      if (!result.ok) actionError = successMessage;
    } catch (error) { actionError = error instanceof Error ? error.message : "Proxy test failed"; }
  }
  async function deletePool(pool: ProxyPool) {
    if (!window.confirm("Delete proxy pool " + pool.name + "?")) return;
    try { await request("/api/proxy-pools/" + encodeURIComponent(pool.id), { method: "DELETE" }); await loadPools(); successMessage = "Proxy pool deleted"; }
    catch (error) { actionError = error instanceof Error ? error.message : "Could not delete proxy pool"; }
  }

  // The shell router owns navigation, so section loading follows the URL instead of a click handler.
  let loadedSection: Section | null = null;
  $effect(() => {
    const section = active;
    if (section === loadedSection) return;
    loadedSection = section;
    actionError = "";
    successMessage = "";
    if (section !== "Console Log") stopConsoleStream();
    if (authenticated) void refreshSection(section);
  });

  onMount(() => {
    const known = [...(Object.values(sectionPaths) as string[]), "/dashboard", "/"];
    if (!known.includes(window.location.pathname)) navigate(sectionPaths["Endpoint & Key"], true);
    void checkSession();
    return () => { stopConsoleStream(); };
  });
</script>

<svelte:head><title>{pageTitle(active)} · Blackhand</title></svelte:head>

<DashboardLayout onLogout={logout}>
  {#if actionError}<div class="message error">{actionError}</div>{/if}
  {#if successMessage}<div class="message success">{successMessage}</div>{/if}

  {#if active === "Endpoint & Key"}
    <EndpointPage />
  {:else if active === "Providers"}
    <div class="panel provider-card"><div class="provider-logo">oc</div><div class="provider-info"><div class="provider-title"><h2>OpenCode Free</h2><span class="pill good">INITIAL PROVIDER</span></div><p>Public model prefix <code>oc/</code>. Requests are translated to native upstream model IDs.</p><div class="provider-meta"><span><i class="status-dot online"></i> Dynamic model catalogue</span><span>Source: opencode.ai/zen/v1/models</span></div></div><button class="button subtle" onclick={loadModels}>Refresh models</button></div>
    <div class="panel endpoint-panel">
      <div class="panel-heading"><h2>Model catalogue</h2><span class="pill">{models.length} models</span></div>
      <label class="field-label" for="provider-key">Client API key <span>Only required when requireApiKey is enabled</span></label>
      <div class="inline-form"><input id="provider-key" type="password" bind:value={probeKey} placeholder="sk-…" autocomplete="off" /><button class="button primary" onclick={loadModels}>Load models</button></div>
      <p class="muted">{modelProbeMessage}</p>
      <div class="model-list">{#each models as model (model.id)}<div class="model-item"><code>{model.id}</code><button class="button subtle" onclick={() => copy(model.id)}>Copy ID</button></div>{/each}</div>
    </div>
  {:else if active === "Usage"}
    <div class="toolbar"><label>Period <select bind:value={period} onchange={() => changePeriod(period)}><option value="today">Today</option><option value="24h">Last 24 hours</option><option value="7d">7 days</option><option value="30d">30 days</option><option value="60d">60 days</option><option value="all">All time</option></select></label><button class="button subtle" onclick={loadUsage}>Refresh</button></div>
    <div class="stat-grid">
      <article class="stat-card"><div class="stat-top">REQUESTS</div><div class="stat-value">{num(stats.totalRequests)}</div><div class="stat-foot">Request records in selected period</div></article>
      <article class="stat-card"><div class="stat-top">INPUT TOKENS</div><div class="stat-value">{num(stats.totalPromptTokens)}</div><div class="stat-foot">Prompt tokens</div></article>
      <article class="stat-card"><div class="stat-top">OUTPUT TOKENS</div><div class="stat-value">{num(stats.totalCompletionTokens)}</div><div class="stat-foot">Completion tokens · Cached {num(stats.totalCachedTokens)}</div></article>
    </div>
    <div class="panel endpoint-panel">
      <div class="panel-heading"><h2>Traffic overview</h2><span class="muted">Tokens per time bucket</span></div>
      {#if chart.length > 0}
        {@const maxTokens = Math.max(1, ...chart.map(p => p.tokens))}
        <div class="chart-bars">{#each chart as point}<div class="chart-col" title={point.label + ": " + num(point.tokens) + " tokens"}><div class="chart-bar" style={"height:" + percentWidth(point.tokens,maxTokens) + "%"}></div><small>{point.label}</small></div>{/each}</div>
      {:else}<p class="muted">No usage data for this period.</p>{/if}
    </div>
    <div class="panel endpoint-panel"><div class="panel-heading"><h2>By provider</h2></div>
      {#each Object.entries(stats.byProvider || {}) as [provider, value]}<div class="provider-stat"><strong>{provider}</strong><span>{num(value.requests)} requests</span><span>{num(value.promptTokens)} in / {num(value.completionTokens)} out</span></div>{/each}
    </div>
    <div class="panel endpoint-panel"><div class="panel-heading"><h2>Recent requests</h2></div>
      <div class="table-wrap"><table><thead><tr><th>Time</th><th>Provider</th><th>Model</th><th>Status</th><th>Tokens</th></tr></thead><tbody>
      {#each (stats.recentRequests || []) as item, i}<tr><td>{item.timestamp || "—"}</td><td>{item.provider || "opencode"}</td><td><code>{item.model || "—"}</code></td><td>{item.status || "ok"}</td><td>{num((item.promptTokens || 0)+(item.completionTokens || 0))}</td></tr>{/each}
      </tbody></table></div>
    </div>
  {:else if active === "Token Saver"}
    <div class="panel endpoint-panel"><div class="panel-heading"><div><div class="eyebrow">REQUEST PIPELINE</div><h2>Token reduction</h2></div><span class="pill">FAIL-OPEN</span></div>
      <p>Local RTK filters run inside Go. Headroom is an optional external compression service; a failed service leaves the original request intact. PXPipe is not automatically installed by this single-binary build.</p>
      <div class="setting-list">
        <label class="setting-row"><span><strong>RTK filters</strong><small>Compact tool output such as git, grep, directory listings and build logs.</small></span><input type="checkbox" checked={settings.rtkEnabled !== false} onchange={(e) => saveSetting("rtkEnabled", (e.currentTarget as HTMLInputElement).checked)} /></label>
        <label class="setting-row"><span><strong>Headroom</strong><small>Send eligible OpenAI message arrays to a compatible Headroom service.</small></span><input type="checkbox" checked={Boolean(settings.headroomEnabled)} onchange={(e) => saveSetting("headroomEnabled", (e.currentTarget as HTMLInputElement).checked)} /></label>
        <label class="setting-row"><span><strong>Compress user messages</strong><small>Ask Headroom to include user messages in compression.</small></span><input type="checkbox" checked={settings.headroomCompressUserMessages !== false} onchange={(e) => saveSetting("headroomCompressUserMessages", (e.currentTarget as HTMLInputElement).checked)} /></label>
        <label class="setting-row"><span><strong>Caveman prompt</strong><small>Encourage shorter, direct response wording.</small></span><input type="checkbox" checked={Boolean(settings.cavemanEnabled)} onchange={(e) => saveSetting("cavemanEnabled", (e.currentTarget as HTMLInputElement).checked)} /></label>
        <label class="setting-row"><span><strong>Ponytail prompt</strong><small>Prefer simpler code and avoid unnecessary abstractions.</small></span><input type="checkbox" checked={Boolean(settings.ponytailEnabled)} onchange={(e) => saveSetting("ponytailEnabled", (e.currentTarget as HTMLInputElement).checked)} /></label>
        <label class="setting-row"><span><strong>PXPipe</strong><small>Configuration is preserved; execution is skipped unless a bundled adapter is added.</small></span><input type="checkbox" checked={Boolean(settings.pxpipeEnabled)} onchange={(e) => saveSetting("pxpipeEnabled", (e.currentTarget as HTMLInputElement).checked)} /></label>
      </div>
      <div class="field-grid"><label class="field"><span>Headroom URL</span><input bind:value={settings.headroomUrl} placeholder="http://localhost:8787" /></label><label class="field"><span>Timeout (ms)</span><input type="number" min="100" max="32000" bind:value={settings.headroomTimeoutMs} /></label></div>
      <button class="button primary" onclick={async () => { await saveSetting("headroomUrl", settings.headroomUrl); await saveSetting("headroomTimeoutMs", Number(settings.headroomTimeoutMs)); }}>Save Headroom options</button>
    </div>
  {:else if active === "Console Log"}
    <div class="panel endpoint-panel"><div class="panel-heading"><div><div class="eyebrow">LIVE DIAGNOSTICS</div><h2>Console log</h2></div><span class="pill good">SSE</span></div>
      <p>Bounded in-memory buffer. Credentials and full request payloads are not intentionally emitted by the router log.</p>
      <div class="console-toolbar"><span>{logs.length} lines buffered</span><button class="button subtle" onclick={loadLogs}>Reload</button><button class="button danger" onclick={clearLogs}>Clear buffer</button></div>
      <div class="console-output">{#each logs as line, i}<div class="console-line"><span>{String(i+1).padStart(3,"0")}</span><code>{line}</code></div>{/each}{#if logs.length===0}<div class="muted">No log lines yet. API requests will appear here.</div>{/if}</div>
    </div>
  {:else if active === "Settings"}
    <ProfilePage />
  {:else if active === "Proxypools"}
    <div class="panel endpoint-panel"><div class="panel-heading"><div><div class="eyebrow">EGRESS</div><h2>Proxy pools</h2></div><span class="pill">{pools.length} pools</span></div>
      <form class="pool-form" onsubmit={createPool}>
        <div class="field-grid"><label class="field"><span>Name</span><input bind:value={poolName} placeholder="Local proxy" required /></label><label class="field"><span>Proxy URL</span><input bind:value={poolUrl} placeholder="http://127.0.0.1:8080" required /></label></div>
        <div class="field-grid"><label class="field"><span>Type</span><select bind:value={poolType}><option value="http">HTTP / HTTPS</option><option value="vercel">Vercel relay</option><option value="cloudflare">Cloudflare relay</option><option value="deno">Deno relay</option><option value="netlify">Netlify relay</option></select></label><label class="field"><span>No-proxy list</span><input bind:value={poolNoProxy} placeholder="localhost,127.0.0.1" /></label></div>
        <label class="setting-row"><span><strong>Strict proxy</strong><small>Do not permit direct-connect fallback when used by a bound connection.</small></span><input type="checkbox" bind:checked={poolStrict} /></label>
        <button class="button primary" type="submit">Add proxy pool</button>
      </form>
    </div>
    <div class="pool-list">
      {#each pools as pool (pool.id)}
        <div class="panel pool-card">
          <div class="pool-main"><div class="provider-logo">⇄</div><div class="provider-info"><div class="provider-title"><h2>{pool.name}</h2><span class:good={pool.isActive} class="pill">{pool.isActive ? "ACTIVE" : "DISABLED"}</span><span class="pill">{pool.testStatus || "untested"}</span></div><p><code>{pool.proxyUrl}</code></p><div class="provider-meta"><span>Type: {pool.type}</span><span>{pool.strictProxy ? "Strict mode" : "Fallback allowed"}</span><span>{pool.boundConnectionCount || 0} bound connections</span></div>{#if pool.lastError}<p class="error-text">{pool.lastError}</p>{/if}</div></div>
          <div class="pool-actions"><button class="button subtle" onclick={() => testPool(pool)}>Test</button><button class="button subtle" onclick={() => togglePool(pool)}>{pool.isActive ? "Disable" : "Enable"}</button><button class="button danger" onclick={() => deletePool(pool)}>Delete</button></div>
        </div>
      {/each}
      {#if pools.length===0}<div class="panel empty-state"><div class="empty-icon">⇄</div><h2>No proxy pools</h2><p>Add a pool above. Tests use a bounded timeout and persist their result.</p></div>{/if}
    </div>
  {/if}
</DashboardLayout>

{#if loginRequired && !authenticated}
  <div class="login-overlay">
    <form class="login-card" onsubmit={login}>
      <div class="brand-mark login-mark"><span class="material-symbols-outlined">hub</span></div>
      <div class="eyebrow">BLACKHAND / SECURE ACCESS</div>
      <h1>Sign in</h1>
      <p>Sign in to manage your endpoint, providers, API keys, usage and router settings.</p>
      {#if authError}<div class="message error">{authError}</div>{/if}
      <label class="field"><span>Dashboard password</span><input type="password" bind:value={password} autocomplete="current-password" required /></label>
      <button class="button primary login-button" type="submit" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      <small>Keep the dashboard bound to localhost unless remote access is protected by a custom password and trusted TLS termination.</small>
    </form>
  </div>
{/if}
