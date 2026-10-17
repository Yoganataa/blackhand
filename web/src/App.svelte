<script lang="ts">
  type Section = "Overview" | "Endpoint & Key" | "Providers" | "Usage" | "Token Saver" | "Console Log" | "Settings" | "Proxypools";
  const sections: { name: Section; icon: string; description: string }[] = [
    { name: "Overview", icon: "◫", description: "Runtime status and quick checks" },
    { name: "Endpoint & Key", icon: "⌘", description: "API endpoint and access keys" },
    { name: "Providers", icon: "◇", description: "OpenCode Free and model discovery" },
    { name: "Usage", icon: "▥", description: "Request and token accounting" },
    { name: "Token Saver", icon: "⌁", description: "Compression pipeline and controls" },
    { name: "Console Log", icon: "≋", description: "Runtime events and diagnostics" },
    { name: "Settings", icon: "⚙", description: "Runtime configuration" },
    { name: "Proxypools", icon: "⇄", description: "Outbound proxy pools" }
  ];
  let active: Section = "Overview";
  let health: "checking" | "online" | "offline" = "checking";
  let endpoint = `${window.location.origin}/v1`;
  let key = "";
  let copied = false;
  async function checkHealth() {
    try {
      const response = await fetch("/readyz");
      health = response.ok ? "online" : "offline";
    } catch { health = "offline"; }
  }
  async function loadModels() {
    const response = await fetch("/v1/models", { headers: key ? { Authorization: `Bearer ${key}` } : {} });
    if (!response.ok) throw new Error(`Model request failed: HTTP ${response.status}`);
    return await response.json();
  }
  async function testModels() {
    try {
      const result = await loadModels();
      alert(`OpenCode model discovery succeeded. Models returned: ${result.data?.length ?? 0}`);
    } catch (error) {
      alert(error instanceof Error ? error.message : "Model discovery failed");
    }
  }
  async function copyEndpoint() {
    try {
      await navigator.clipboard.writeText(endpoint);
      copied = true;
      window.setTimeout(() => copied = false, 1500);
    } catch {
      copied = false;
    }
  }
  checkHealth();
</script>

<svelte:head><title>{active} · Blackhand</title></svelte:head>
<div class="shell">
  <aside class="sidebar">
    <a class="brand" href="#overview" onclick={() => active = "Overview"}>
      <span class="brand-mark">B</span>
      <span><strong>blackhand</strong><small>GO ROUTER</small></span>
    </a>
    <div class="nav-label">WORKSPACE</div>
    <nav>
      {#each sections as section}
        <button class:active={active === section.name} class="nav-item" onclick={() => active = section.name}>
          <span class="nav-icon">{section.icon}</span><span>{section.name}</span>
          {#if section.name === "Providers"}<span class="nav-count">1</span>{/if}
        </button>
      {/each}
    </nav>
    <div class="sidebar-bottom">
      <div class="status-line"><span class:online={health === "online"} class:offline={health === "offline"} class="status-dot"></span>{health === "online" ? "Runtime ready" : health === "offline" ? "Runtime unavailable" : "Checking runtime"}</div>
      <small>Single-binary architecture</small>
    </div>
  </aside>
  <main class="main">
    <header class="topbar">
      <div class="breadcrumbs"><span>Workspace</span><span class="slash">/</span><strong>{active}</strong></div>
      <div class="top-actions"><span class="version-chip">GO · SPA</span><button class="icon-button" aria-label="Refresh status" onclick={checkHealth}>↻</button><span class="avatar">BH</span></div>
    </header>
    <section class="content">
      <div class="page-head">
        <div><div class="eyebrow">BLACKHAND / ROUTER</div><h1>{active}</h1><p>{sections.find(s => s.name === active)?.description}</p></div>
        <button class="button subtle" onclick={checkHealth}>↻ Refresh status</button>
      </div>
      {#if active === "Overview"}
        <div class="stat-grid">
          <article class="stat-card"><div class="stat-top">RUNTIME STATUS <span class="tiny-icon">◉</span></div><div class="stat-value">{health === "online" ? "Ready" : health === "offline" ? "Offline" : "Checking"}</div><div class="stat-foot"><span class:good={health === "online"} class="pill">{health === "online" ? "HEALTHY" : health.toUpperCase()}</span><span>Go process</span></div></article>
          <article class="stat-card"><div class="stat-top">ACTIVE PROVIDERS <span class="tiny-icon">◇</span></div><div class="stat-value">01</div><div class="stat-foot"><span class="pill">OPEN SOURCE</span><span>OpenCode Free</span></div></article>
          <article class="stat-card"><div class="stat-top">PUBLIC API <span class="tiny-icon">⌘</span></div><div class="stat-value">/v1</div><div class="stat-foot"><span class="pill">OPENAI COMPATIBLE</span><span>Port 20128</span></div></article>
        </div>
        <div class="panel endpoint-panel">
          <div class="panel-heading"><div><div class="eyebrow">CONNECTION</div><h2>Connect your coding tool</h2></div><span class="live-label"><i></i> Local endpoint</span></div>
          <p>Use this endpoint in a compatible AI coding client. Add your configured API key if authentication is enabled.</p>
          <div class="endpoint-row"><code>{endpoint}</code><button class="button primary" onclick={copyEndpoint}>{copied ? "Copied" : "Copy endpoint"}</button></div>
          <label class="field-label" for="api-key">API key <span>Optional if server auth is disabled</span></label>
          <input id="api-key" type="password" bind:value={key} placeholder="Enter configured API key to test model discovery" autocomplete="off" />
          <div class="panel-footer"><button class="button subtle" onclick={testModels}>Test model endpoint</button><span>Credentials are not stored by this page.</span></div>
        </div>
      {:else if active === "Providers"}
        <div class="panel provider-card"><div class="provider-logo">oc</div><div class="provider-info"><div class="provider-title"><h2>OpenCode Free</h2><span class="pill good">INITIAL PROVIDER</span></div><p>Dynamic model discovery · Prefix <code>oc/</code> · No account setup in this scaffold</p><div class="provider-meta"><span><i class="status-dot online"></i> Model discovery on demand</span><span>Source: opencode.ai/zen/v1/models</span></div></div><button class="button subtle" onclick={testModels}>Refresh models</button></div>
        <div class="notice"><strong>Porting status</strong><p>Provider discovery and chat proxy are wired in the Go scaffold. Model list UI, usage persistence and exhaustive compatibility checks are still being implemented.</p></div>
      {:else}
        <div class="panel empty-state"><div class="empty-icon">{sections.find(s => s.name === active)?.icon}</div><h2>{active} module scaffolded</h2><p>This area is part of the porting scope. Its API contract and persisted behavior must be matched against 9Router before marking it complete.</p><span class="pill">PARITY WORK PENDING</span></div>
      {/if}
      <div class="bottom-note"><span>BLACKHAND</span><span>Built for predictable performance and portable releases</span></div>
    </section>
  </main>
</div>
