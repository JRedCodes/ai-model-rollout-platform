// Build-time API base URL. Defaults to "/api", which only resolves today
// via Vite's dev server proxy (vite.config.ts) -- a production build
// served from a different origin than the API (e.g. CloudFront + a
// separate ALB, see documents/DEPLOYMENT.md) has no equivalent proxy and
// needs this set to the real absolute API origin at build time.
export const API_BASE = import.meta.env.VITE_API_URL ?? "/api";
