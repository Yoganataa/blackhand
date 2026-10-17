// Ported from 9Router `src/shared/utils/cn.js` (decolua/9router@87a74e1): join conditional
// classes, collapse whitespace. Upstream does not merge conflicting Tailwind classes either.
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts
    .filter(Boolean)
    .join(" ")
    .replace(/\s+/g, " ")
    .trim();
}
