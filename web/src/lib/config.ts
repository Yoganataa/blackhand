// Shell constants. Upstream reads these from `@/shared/constants/config` (APP_CONFIG).
// ponytail: version is a constant for now; wire it to the server's version endpoint when the
// Setting area (which owns /api/version) is ported.
export const APP_CONFIG = {
  name: "Blackhand",
  version: "0.1.0",
};
