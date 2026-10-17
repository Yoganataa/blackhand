type ThemeValue = "light" | "dark" | "system";

// Theme state. The pre-paint script in index.html reads the same localStorage key, so the first
// paint already matches the stored value.
const STORAGE_KEY = "theme";

function readStored(): ThemeValue {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw === "dark" || raw === "light" || raw === "system") return raw;
  } catch {
    // storage blocked; fall through to the default
  }
  return "system";
}

function prefersDark(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export const theme = $state<{ value: ThemeValue }>({
  value: typeof window === "undefined" ? "system" : readStored(),
});

export function isDark(): boolean {
  return theme.value === "dark" || (theme.value !== "light" && prefersDark());
}

function apply(): void {
  if (typeof document === "undefined") return;
  document.documentElement.classList.toggle("dark", isDark());
}

export function setTheme(value: ThemeValue): void {
  theme.value = value;
  try {
    window.localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // storage blocked; the session still applies the theme
  }
  apply();
}

export function toggleTheme(): void {
  setTheme(isDark() ? "light" : "dark");
}

if (typeof window !== "undefined") {
  apply();
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (theme.value === "system") apply();
  });
}
