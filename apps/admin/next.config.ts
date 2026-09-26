import path from "node:path";
import type { NextConfig } from "next";
import { loadEnvConfig } from "@next/env";
import createNextIntlPlugin from "next-intl/plugin";

// One .env at the repository root serves every service (.env.example); Next itself only reads this app's folder.
loadEnvConfig(path.resolve(process.cwd(), "../.."), process.env.NODE_ENV !== "production", undefined, true);

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");

const nextConfig: NextConfig = {
  // `next dev` would otherwise write its own AGENTS.md / CLAUDE.md here; the project's live at the repo root.
  agentRules: false,
  transpilePackages: ["@pdpa/api-client", "@pdpa/authz", "@pdpa/i18n", "@pdpa/ui"],
};

export default withNextIntl(nextConfig);
