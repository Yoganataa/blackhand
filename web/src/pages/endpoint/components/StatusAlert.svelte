<script lang="ts">
  // Ported from 9Router `dashboard/endpoint/components/StatusAlert.js`.
  import { cn } from "../../../lib/cn";

  let {
    status,
    className = "",
  }: { status: { type: "success" | "warning" | "info" | "error"; message: string }; className?: string } = $props();

  const containerClass = $derived(
    cn(
      "p-2 rounded text-sm",
      className,
      status.type === "success"
        ? "bg-green-500/10 text-green-600 dark:text-green-400"
        : status.type === "warning"
          ? "bg-yellow-500/10 text-yellow-600 dark:text-yellow-400"
          : status.type === "info"
            ? "bg-blue-500/10 text-blue-600 dark:text-blue-400"
            : "bg-red-500/10 text-red-600 dark:text-red-400",
    ),
  );

  // Upstream linkifies the first http(s) URL found in the message.
  const parts = $derived(status.message.split(/(https?:\/\/[^\s]+)/g));
</script>

<div class={containerClass}>
  {#each parts as part, i (i)}
    {#if /^https?:\/\//.test(part)}
      <a href={part} target="_blank" rel="noreferrer" class="underline font-medium">{part}</a>
    {:else}{part}{/if}
  {/each}
</div>
