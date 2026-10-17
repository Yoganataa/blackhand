<script lang="ts">
  // Ported from 9Router `src/shared/components/ProviderIcon.js` (decolua/87a74e1).
  // Upstream consults a registry of missing icons; Blackhand ships only the assets it ports, so a
  // failed load falls back to the provider initials.
  let {
    src,
    providerId,
    alt,
    size = 32,
    className = "",
    fallbackText = "?",
    fallbackColor,
  }: {
    src?: string;
    providerId?: string;
    alt?: string;
    size?: number;
    className?: string;
    fallbackText?: string;
    fallbackColor?: string;
  } = $props();

  let errored = $state(false);

  const effectiveSrc = $derived.by(() => {
    if (providerId) return `/providers/${providerId}.png`;
    if (!src) return null;
    const match = String(src).match(/^\/providers\/([^/]+)\.png$/i);
    return match ? `/providers/${match[1]}.png` : src;
  });

  const fontSize = $derived(Math.max(10, Math.floor(size * 0.38)));
</script>

{#if !effectiveSrc || errored}
  <span
    class="inline-flex items-center justify-center font-bold rounded-lg {className}"
    style="width:{size}px;height:{size}px;color:{fallbackColor};font-size:{fontSize}px"
  >
    {fallbackText}
  </span>
{:else}
  <img
    src={effectiveSrc}
    alt={alt || ""}
    width={size}
    height={size}
    class={className}
    loading="lazy"
    decoding="async"
    onerror={() => (errored = true)}
  />
{/if}
