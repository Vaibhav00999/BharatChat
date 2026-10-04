# Contributing

This repository contains an engineering preview, not a public messaging release.
Read `SECURITY.md`, `docs/PRIVACY.md`, and `docs/LAUNCH_ROADMAP.md` first. GitHub
publication and passing tests are not security approval.

## Local Setup

Follow the root README for Go, Docker, and Flutter setup. Start from
`backend/.env.example` and keep your local environment file untracked. Never use
production credentials or send test OTPs to people who have not consented.

## Verification

- Backend: formatting, module verification, vet, race tests including every
  container integration, and a production image build. On Windows, run
  `backend/scripts/verify-container.ps1` with a healthy local Linux Docker engine.
- Flutter: `flutter pub get`, `dart format --output=none --set-exit-if-changed lib test`,
  `flutter analyze`, `flutter test`, and a release build without plaintext flags.
- Native MLS component: use its separate README and locked dependencies. It is
  experimental and must not be connected to public messaging without review.
- Deployment: run the mocked PowerShell tests and configuration checks from the
  AWS runbook. They do not require AWS credentials or create cloud resources.

Keep changes scoped, add regression tests, and describe verification and
remaining limitations in each pull request. Do not claim an unrun test passed.
Do not commit environment files, credentials, database files, device signing
keys, binary builds, local logs, or runtime deployment evidence.

Cryptographic protocol, membership, backup/recovery and privacy changes require
explicit design review. Do not implement a custom cipher or silently fall back
to plaintext. Hosting remains paused until the owner explicitly resumes it.
