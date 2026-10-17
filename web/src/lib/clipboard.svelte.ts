// Ported from 9Router `src/shared/hooks/useCopyToClipboard.js` (decolua/87a74e1):
// `copied` holds the id of the last copied item and resets after a delay.
export function createClipboard(resetDelay = 2000) {
  let copied = $state<string | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function copy(text: string, id = "default") {
    const write = async () => {
      if (navigator?.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return;
      }
      const textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
    };
    void write().catch(() => {});
    copied = id;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => (copied = null), resetDelay);
  }

  return {
    get copied() {
      return copied;
    },
    copy,
  };
}
