# Contributing to Cloudreve-New

Thanks for helping improve Cloudreve-New.

## Ground rules

- Licensed under **GPL-3.0**. By contributing you agree your changes may be
  distributed under the same terms.
- Never submit code, binaries, bundles, or assets originating from the
  proprietary Pro edition.
- Never commit credentials, configuration files with secrets, production
  database dumps, or user data.

## Workflow

1. Open an issue describing the change and, for bugs, the reproduction steps.
2. Fork the repo, branch from `master`, and keep each PR focused.
3. Backend: `cd src && go build ./... && go test ./...`
4. Frontend: `cd web && pnpm install && pnpm lint && pnpm build`
5. Explain how you verified the change; include screenshots for UI work.

## Layout

| Path | Contents |
| --- | --- |
| `src/` | Go backend, ent schema, routers, services |
| `web/` | React + TypeScript frontend |
| `tools/` | End-to-end PowerShell/Node test scripts (credentials via env vars) |

If a change spans API and UI, update both in the same PR.

## Security

Do not open public issues for vulnerabilities — see [SECURITY.md](SECURITY.md).
