# Launch Roadmap

Status: engineering preview, not a public messaging service. Infrastructure
configuration and passing tests are necessary but do not establish production
security, delivery guarantees, or stronger privacy than other messengers.

| Priority | Work | Acceptance evidence |
| --- | --- | --- |
| P0 | Audited client encryption integration | Isolated native OpenMLS/SQLCipher component passes 14 Windows tests; not integrated into the app. Still required: every shipped platform, authenticated key packages and identity verification, multi-device transport, ordered membership commits, recovery, interoperability and independent review. No plaintext fallback in release builds. |
| P0 | AWS and MSG91 staging | Authorized AWS access, Mumbai resources, private TLS datastores, scoped role, certificate renewal, approved MSG91 template and owned-handset delivery tests. Configuration exists in `deploy/aws`; live deployment is not done. |
| P0 | Message recovery and account lifecycle | Encrypted, user-scoped client outbox, stable idempotency IDs, reconnect/process-restart resend, acknowledgement cleanup, REST refresh after reconnect, and logout erasure are implemented. Still required: device-loss recovery policy, multi-device convergence, long-offline/load drills, and end-to-end failure testing with the audited cipher. |
| P0 | Abuse and privacy operations | Export UI, bounded snapshot archives, session-confirmed deletion, transactional credential recheck, owned-group protection, OTP erasure, and outbox release-lock regression tests are implemented. Real PostgreSQL export/deletion tests passed on 2026-10-01 after restoring Docker, including expired/deleted-message metadata and former-member group-info exclusion. Staffed report handling, rate/spend alarms, backup/object erasure, multi-device deletion, recovery policy, and reviewed privacy disclosures remain required. |
| P1 | Release and resilience | Signed Android/iOS artifacts on physical devices, browser compatibility, accessibility and slow-network tests, race-enabled CI, dependency/image scans, backup restores, load tests, and multi-instance failover drills. |
| P1 | Controlled beta | Closed cohort only after P0 gates pass; monitor delivery latency, reconnect success, crash-free sessions, SMS failures, report response time, and deletion completion. Defined rollback and incident ownership. |
| P2 | Product expansion | Encrypted media, notifications without sensitive content, contacts with consent, calls, and optional differentiating workflows. Add these after reliable private messaging works end to end. |

The practical initial distinction is a privacy-focused experience: exact-username
discovery, restrained metadata collection, clear device control, and understandable
safety actions. Validate those benefits with beta users; do not advertise unproven
comparisons with WhatsApp, Telegram, or Signal.

See [production verification and blockers](PRODUCTION_READINESS.md) for measured
results, and the [AWS runbook](../deploy/aws/README.md) for deployment prerequisites.
