# Contributing to Cloudreve-New

Thanks for your interest in improving Cloudreve-New.

## Before you start

- This repository is an unofficial fork of Cloudreve based on the **community
  source tree**. It is licensed under GPL-3.0; by submitting a contribution you
  agree it can be distributed under the same license.
- Do not submit code copied from the proprietary Pro edition, and do not attach
  Pro binaries, bundles or decompiled artifacts.
- Never commit credentials, production database dumps, `data/` directories or
  configuration files containing secrets.

## Workflow

1. Open an issue describing the bug or proposal, including reproduction steps.
2. Fork, create a topic branch, and keep each PR scoped to one change.
3. Match the surrounding code style; `gofmt` for Go, and the existing
   Prettier/ESLint setup in `web/`.
4. Add or update tests when behavior changes. Backend tests live next to the
   packages (`go test ./...`), frontend checks run via `pnpm lint`.
5. Describe verification steps in the PR body. Screenshots help for UI changes.

## Reporting security issues

Please do not open public issues for vulnerabilities. See [SECURITY.md](SECURITY.md).
