<script lang="ts">
  // Ported from 9Router `src/shared/components/layouts/DashboardLayout.js` (decolua/9router@87a74e1).
  // Deferred: the idle-time preloading of the usage charts (React-only concern) and the
  // basic-chat full-height variant (that page is out of scope).
  import type { Snippet } from "svelte";
  import { notifications, removeNotification } from "../lib/notifications.svelte";
  import Header from "./Header.svelte";
  import Sidebar from "./Sidebar.svelte";

  let { children, onLogout }: { children: Snippet; onLogout: () => void } = $props();

  let sidebarOpen = $state(false);

  function getToastStyle(type: string) {
    if (type === "success") {
      return { wrapper: "border-green-500/30 bg-green-500/10 text-green-600 dark:text-green-400", icon: "check_circle" };
    }
    if (type === "error") {
      return { wrapper: "border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400", icon: "error" };
    }
    if (type === "warning") {
      return { wrapper: "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400", icon: "warning" };
    }
    return { wrapper: "border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400", icon: "info" };
  }
</script>

<div class="flex h-screen w-full overflow-hidden bg-bg">
  <!-- Toasts -->
  <div class="fixed top-4 right-4 z-[80] flex w-[min(92vw,380px)] flex-col gap-2">
    {#each notifications.items as notification (notification.id)}
      {@const style = getToastStyle(notification.type)}
      <div class="rounded-lg border px-3 py-2 shadow-lg backdrop-blur-sm {style.wrapper}">
        <div class="flex items-start gap-2">
          <span class="material-symbols-outlined text-[18px] leading-5">{style.icon}</span>
          <div class="min-w-0 flex-1">
            {#if notification.title}
              <p class="text-xs font-semibold mb-0.5">{notification.title}</p>
            {/if}
            <p class="text-xs whitespace-pre-wrap break-words">{notification.message}</p>
          </div>
          {#if notification.dismissible}
            <button
              type="button"
              onclick={() => removeNotification(notification.id)}
              class="text-current/70 hover:text-current"
              aria-label="Dismiss notification"
            >
              <span class="material-symbols-outlined text-[16px]">close</span>
            </button>
          {/if}
        </div>
      </div>
    {/each}
  </div>

  <!-- Mobile sidebar overlay -->
  {#if sidebarOpen}
    <button
      type="button"
      class="fixed inset-0 z-40 bg-black/20 lg:hidden"
      aria-label="Close sidebar"
      onclick={() => (sidebarOpen = false)}
    ></button>
  {/if}

  <!-- Sidebar - desktop -->
  <div class="hidden lg:flex">
    <Sidebar />
  </div>

  <!-- Sidebar - mobile -->
  <div
    class="fixed inset-y-0 left-0 z-50 transform lg:hidden transition-transform duration-300 ease-in-out {sidebarOpen
      ? 'translate-x-0'
      : '-translate-x-full'}"
  >
    <Sidebar onClose={() => (sidebarOpen = false)} />
  </div>

  <!-- Main content -->
  <main class="flex flex-col flex-1 h-full min-w-0 relative transition-colors duration-300 isolate">
    <div class="landing-grid absolute inset-0 pointer-events-none -z-10" aria-hidden="true"></div>
    <Header onMenuClick={() => (sidebarOpen = true)} {onLogout} />
    <div class="flex-1 overflow-y-auto custom-scrollbar p-6 lg:p-10">
      <div class="max-w-7xl mx-auto">
        {@render children()}
      </div>
    </div>
  </main>
</div>
