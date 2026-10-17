// Provider registry subset. Only OpenCode Free is in scope for now (docs/PORT_1TO1.md); the values
// mirror 9Router `open-sse/providers/registry/opencode.js` and `src/shared/constants/providers.js`
// (category `freeTier`, `noAuth: true`, colour #E87040, text icon "OC", alias `oc`).
export type ProviderInfo = {
  id: string;
  name: string;
  color: string;
  textIcon: string;
  alias: string;
  noAuth?: boolean;
  category: "freeTier";
};

export const PROVIDERS: Record<string, ProviderInfo> = {
  opencode: {
    id: "opencode",
    name: "OpenCode Free",
    color: "#E87040",
    textIcon: "OC",
    alias: "oc",
    noAuth: true,
    category: "freeTier",
  },
};

export const FREE_TIER_PROVIDERS = Object.values(PROVIDERS).filter((provider) => provider.category === "freeTier");

export function providerInfo(providerId: string): ProviderInfo | undefined {
  return PROVIDERS[providerId];
}

// Which catalogue models count as the provider's free models, mirrored from 9Router
// `api/providers/suggested-models/filters.js` → the `opencode-free` filter: an id ending in `-free`
// or one of the known-free ids, minus the ids upstream currently treats as dead.
const KNOWN_FREE_OPENCODE_MODELS = ["big-pickle"];
const DEAD_FREE_OPENCODE_MODELS = ["deepseek-v4-flash-free"];

export function isFreeOpenCodeModel(id: string): boolean {
  const base = id.split("/").pop() ?? id;
  if (DEAD_FREE_OPENCODE_MODELS.includes(base)) return false;
  return base.endsWith("-free") || KNOWN_FREE_OPENCODE_MODELS.includes(base);
}
