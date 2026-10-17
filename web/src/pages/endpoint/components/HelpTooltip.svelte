<script lang="ts">
  // Ported from 9Router `dashboard/endpoint/components/Tooltip.js` (the page-local help bubble).
  let { text }: { text: string } = $props();
</script>

<span class="relative group inline-flex items-center">
  <span class="material-symbols-outlined text-[14px] text-text-muted cursor-help">help</span>
  <span
    class="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 z-50 w-64 rounded bg-gray-900 dark:bg-gray-800 text-white text-xs px-2.5 py-1.5 opacity-0 group-hover:opacity-100 transition-opacity shadow-lg"
    >{text}</span
  >
</span>
