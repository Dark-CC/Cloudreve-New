/**
 * Central place for outbound links and the optional Giscus announcement board.
 *
 * Override any of these at build time, e.g.
 *   VITE_REPO_URL=https://github.com/you/Cloudreve-New pnpm build
 */
const env = import.meta.env as Record<string, string | undefined>;

const repo = env.VITE_REPO_URL?.replace(/\/+$/, "") ?? "https://github.com/cloudreve-new/cloudreve-new";

type RepoPath = `${string}/${string}`;

export const BRANDING = {
  name: env.VITE_BRAND_NAME ?? "Cloudreve",
  repoUrl: repo,
  homeUrl: env.VITE_HOME_URL ?? repo,
  docsUrl: env.VITE_DOCS_URL ?? `${repo}#readme`,
  discussionUrl: env.VITE_DISCUSSION_URL ?? `${repo}/discussions`,
  discordUrl: env.VITE_DISCORD_URL ?? "",
  telegramUrl: env.VITE_TELEGRAM_URL ?? "",
  giscus: {
    repo: (env.VITE_GISCUS_REPO ?? "") as RepoPath,
    repoId: env.VITE_GISCUS_REPO_ID ?? "",
    category: env.VITE_GISCUS_CATEGORY ?? "",
    categoryId: env.VITE_GISCUS_CATEGORY_ID ?? "",
  },
};

export const giscusEnabled = () => Boolean(BRANDING.giscus.repo && BRANDING.giscus.repoId);
