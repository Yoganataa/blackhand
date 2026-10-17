// Provider registry subset. Only OpenCode Free is in scope for now (docs/PORT_1TO1.md); the values
// mirror 9Router `open-sse/providers/registry/opencode.js` and `src/shared/constants/providers.js`
// (category `freeTier`, `noAuth: true`, colour #E87040, text icon "OC").
export type ProviderInfo = {
  id: string;
  name: string;
  color: string;
  textIcon: string;
  noAuth?: boolean;
  category: "freeTier";
};

export const PROVIDERS: Record<string, ProviderInfo> = {
  opencode: {
    id: "opencode",
    name: "OpenCode Free",
    color: "#E87040",
    textIcon: "OC",
    noAuth: true,
    category: "freeTier",
  },
};

export const FREE_TIER_PROVIDERS = Object.values(PROVIDERS).filter((provider) => provider.category === "freeTier");

export function providerInfo(providerId: string): ProviderInfo | undefined {
  return PROVIDERS[providerId];
}
