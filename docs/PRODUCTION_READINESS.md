# Production Readiness

This repository now fails closed on the production settings it can enforce, but
it is not approved for public users until every launch gate below is complete.

## Enforced Production Configuration

- `APP_ENV=production`
- `JWT_ACCESS_SECRET` is unique, random, and at least 32 characters
- `JWT_ISSUER` and `JWT_AUDIENCE` identify this deployment and its clients
- `POSTGRES_SSLMODE=verify-full` with authenticated database access
- `REDIS_TLS=true` with authenticated Redis access
- `CORS_ALLOWED_ORIGINS` contains only the deployed HTTPS web origins
- `TRUSTED_PROXIES` contains only the ingress or load-balancer CIDRs
- `REQUIRE_E2EE=true`
- `MESSAGING_ENABLED=false`: non-development configuration refuses to enable
  sending until the E2EE launch gate is reviewed in a future release
- `OTP_ALLOWED_PHONES`: 1-20 unique, canonical E.164 test numbers; both OTP sends
  and verification are restricted to this list
- `OTP_DELIVERY_MODE=webhook` with an authenticated HTTPS adapter, or
  `OTP_DELIVERY_MODE=twilio` with API-key credentials, a Messaging Service, and
  the approved SMS template, or `OTP_DELIVERY_MODE=msg91` with its dedicated auth
  key and approved OTP template ID as described in `docs/OTP_DELIVERY.md`

Selected provider: MSG91. Selected domain: `bharatchat.in`.
Credentials, DNS/hosting, and handset SMS delivery have not yet been verified.
Selected host: Ubuntu EC2 in AWS Mumbai (`ap-south-1`), Docker, and Nginx.
The [AWS deployment bundle](../deploy/aws/README.md) is prepared and locally tested,
but no EC2 host or production application has been provisioned. A dedicated IAM
onboarding user, `bharatchat-deployer`, was created without access keys. Onboarding
permissions are attached and verified. On 2026-09-20, live IAM checks confirmed
the owner's console password setup and MFA for the deployer, plus Root MFA and
no Root access keys. After opening approval in the owner's Chrome IAM session,
STS verified the CLI as `bharatchat-deployer`, not Root. The read-only Mumbai
host preflight (`create-host.ps1 -WhatIf`) passed as that user, and the instance
inventory was empty. On 2026-10-02, the restricted provisioning policy and runtime
role/profile were successfully created by the one-time IAM bootstrap described
below. EC2 creation is still pending. Root must not be used for application
deployment.

On 2026-09-22, a separate one-time IAM bootstrap template and restricted host
provisioning policy were prepared and validated locally. The host template no
longer creates IAM resources and now requires a pinned, reviewed Ubuntu AMI.
Offline policy checks and mocked host provisioning tests pass; the bootstrap
template also passes AWS template validation. The policy was subsequently
attached by the successful 2026-10-02 bootstrap; there is still no deployed server.
Authorization codes must not be shared in chat.

## Latest Safety Work

- Account export now has a Flutter consent/download flow and a server-side
  read-only snapshot archive. Refresh hashes, access-token identifiers, push
  tokens, storage object keys, and private keys are excluded. Archives are not
  password-protected and are not message-key backups.
- Export responses are private/no-store, bounded to 32 MiB and 20 seconds, with
  two concurrent exports per server and two requests per account per hour.
  An incomplete export fails before any attachment response is committed.
- Account deletion requires the current session's refresh credential, rechecks
  and locks it inside the erasure transaction, rejects active group owners until
  ownership is transferred, and removes retained OTP records for the account.
  Cleanup of empty chats is scoped to the deleted account's memberships.
- Restored outbox messages now obey the release plaintext lock on startup and
  reconnect. Existing development messages cannot bypass that lock.
- Account archives filter message-linked receipts, reactions, media metadata,
  and notification previews by membership dates, expiry, and deletion state.
  Former members do not receive a group's current name, description, icon, or
  disappearing-message setting through their account export.
- WebSockets revalidate sessions before inbound dispatch and queued outbound
  delivery, with a three-second check context, in addition to idle checks.
  Checks fail closed. Events already in flight cannot be recalled.
- Verification uses Testcontainers 0.44.0 and x/crypto 0.56.0. PostgreSQL test
  fixtures wait for completed initialization, and HTTP fixtures register
  container cleanup immediately. The Windows verification runner supplies the
  Linux C toolchain, checks race tests without accepting skipped tests or stale
  evidence, builds the production image, and runs an isolated startup smoke
  test. CI distinguishes a skipped test
  from a package that has no test files.

- Prelaunch backend messaging is locked for both plaintext and ciphertext. The
  release client composer remains disabled. This does not complete client E2EE.
- OTP allowlists restrict new logins and SMS costs during controlled testing.
  Removing a number does not revoke existing sessions automatically.
- AWS host automation checks the expected account, subnet and Internet Gateway
  route, refuses to overwrite an existing stack, enables termination protection,
  and records actual instance/IP outputs only after a successful creation.
- Ubuntu bootstrap uses SSM rather than SSH and adds Docker-aware firewall rules.
  Scripts have local validation only; no EC2 host has been created or configured.

- Authenticated exact-username discovery returns only ID, display name, and username.
- Block/unblock and paginated blocked-user management are available in Flutter.
- Existing direct messages enforce both-direction blocks at persistence time;
  typing is also denied. Shared groups and historical messages remain accessible.
- Account reporting submits only the selected reason and voluntarily entered
  details. There is no automatic message attachment or completed moderation console.
- Lookup and report submissions have separate per-account rate limits.
- The HTTP integration suite now verifies successful OTP login, single-use codes,
  refresh rotation/reuse rejection, and revocation of both old and new access tokens.
- Frontend chat state and WebSockets are scoped to the signed-in account, and
  rejected/expired sends can be retried with the same idempotency key.

The Compose MinIO service is loopback-bound local-development infrastructure and
must never be promoted into production. Deploy a supported, patched S3-compatible
service with dedicated least-privilege application credentials, encryption at
rest, audit logging, object versioning, and tested recovery instead.

The service exposes `/healthz` for liveness and `/readyz` for PostgreSQL and Redis
readiness. Production access tokens are bound to configured issuer and audience.

## Release Verification

Run these checks on every release candidate:

```bash
cd backend
go mod verify
gofmt -l .
go vet ./...
go test -race ./...
govulncheck ./...
docker build --target production -t bharatchat-backend:candidate .
```

The repository's backend CI performs formatting, vet, race-enabled tests, and a
production binary build. Dependabot tracks Go, Dart, Docker, and GitHub Actions
updates. Image signing, SBOM generation, provenance, and deployment admission
policy still belong in the selected deployment platform.

## Current Verification Snapshot

Additional verification on 2026-10-02:

- The deployer submitted the restricted Mumbai pilot host stack. Security-group
  creation succeeded, but AWS rejected the reviewed `t3.medium` as not Free
  Tier-eligible. The owner explicitly paused hosting rather than changing billing
  or substituting another instance type. AWS completed rollback: both resources
  are `DELETE_COMPLETE`, and actual queries found no BharatChat instances or
  Elastic IPs. The termination-protected failed stack record is preserved;
  evidence is in ignored `deploy/aws/runtime/host-attempt-20261002.json` and
  `host-failed-events-20261002.json`. No retry, deletion, billing change, DNS/TLS
  work or backend deployment was performed. The successful IAM stack is intact.
  Host automation now preserves submission/failure evidence and verifies complete
  status, protection and required outputs before writing a success record;
  its extended mocked failure/identity/dry-run tests pass.
- The first live IAM bootstrap attempt failed and rolled back: its deployer
  inline policy exceeded the IAM user limit of 2,048 characters. The earlier
  offline test incorrectly used the role's 10,240-character limit. The template
  now attaches a restricted customer managed policy with the same permissions
  (5,124/6,144 characters), with separate user-inline and managed-policy quota
  assertions. Corrected lint, tests and AWS template validation pass. After
  verifying all three failed-attempt resources had been deleted by rollback,
  its event history was archived and only that exact failed stack record was
  removed. The corrected stack reached `CREATE_COMPLETE` with termination
  protection enabled; outputs are in ignored
  `deploy/aws/runtime/iam-completed-20261002.json`. IAM simulation allows the
  pilot-stack creation but denies IAM editing, access-key creation and direct
  secret reads. Actual deployer identity and runtime-profile reads succeeded.
  The explicitly authorized one-time Root profile was logged out again, and a
  subsequent STS lookup confirmed its cached login was unavailable. No EC2 host
  was created. Root is not authorized for EC2 or application deployment.
- The isolated `frontend/packages/bharatchat_mls` package passed formatting,
  Dart analysis with zero issues, and all 14 Windows native tests. These use
  real OpenMLS/SQLCipher, covering two-way encryption, tamper/replay rejection,
  group removal/epoch rotation, encrypted persistence, missing/wrong-key state,
  initialization recovery and account/device isolation. It is not imported by
  the shipping app and does not complete client E2EE. Secure-storage integration,
  authenticated KeyPackages, commit/Welcome transport, membership authorization,
  mobile verification and independent review remain launch blockers.
- Frontend CI now includes this isolated package's locked dependency resolution,
  formatting, analysis and native tests. The changed remote workflow has not run;
  Linux, Android and iOS native behavior has not been established locally.
- AWS STS verified the renewed `bharatchat-mumbai-prod` profile as IAM user
  `bharatchat-deployer` in the expected account, not Root. Account-specific
  identifiers are preserved only in ignored local runtime evidence.
  Mumbai EC2 inventory was empty; host read-only preflight passed with the
  current Canonical-owned Ubuntu AMI. Reading the required runtime instance
  profile was initially denied during preflight and now succeeds after the
  corrected IAM bootstrap above. Host provisioning remains pending. No
  EC2 deployment, DNS/TLS changes or live MSG91 sends have been completed.

Latest verification on 2026-10-01 (supersedes older snapshots):

- The Docker Linux engine was restored without resetting application volumes.
  The complete Go suite passed inside Linux with the race detector: 106 test
  cases/subcases passed, zero failed, and zero tests skipped. All PostgreSQL and
  Redis integrations ran, including export filtering, transactional account
  deletion, migrations, OTP login/rotation/revocation, HTTP/WebSocket messaging,
  revoked-connection event rejection, and Redis pub/sub fan-out.
  Evidence is in the ignored `backend/test-results-container.jsonl` artifact.
- Go module integrity and formatting checks and `go vet` passed. Local builds
  require `-buildvcs=false` because workspace VCS metadata is not available.
  Linux race verification uses the trusted local Docker socket and disposable
  test services, with no production credentials.
- The production Docker target built successfully. That exact candidate ran as
  the non-root `bharatchat` user with a read-only filesystem and no capabilities,
  using disposable PostgreSQL/Redis containers on an internal-only network.
  Migrations completed; `/healthz` and `/readyz` returned 200, and unauthenticated
  `/api/v1/users/me` returned 401. Probes used in-container loopback with no
  published ports. All smoke-test containers, their volumes, and the network
  were removed. This used development configuration with messaging disabled;
  it verifies packaging/startup, not production TLS, SMS, AWS, or E2EE.
- A fresh `govulncheck` symbol scan found no reachable vulnerabilities in the
  current code or imported packages after the dependency upgrades. One advisory
  remains in the required but unused, unmaintained OpenPGP package
  ([GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)); no fixed version exists.
  BharatChat does not import that package. Do not introduce it into application
  code. This scan is not a security audit or a clean whole-module-graph claim.
- All 45 Flutter tests passed, including export consent/cancellation, deletion
  confirmation, credential rotation during a deletion retry, and restored-outbox
  release-lock enforcement. Flutter analysis reported zero issues.
  Client formatting also passes after the SDK-required formatter transition.
- Flutter 3.47.5 compiled the JavaScript release web client with an HTTPS API URL
  and no plaintext override. WebAssembly is not supported by the current secure
  storage dependency. Native share sheets and desktop save dialogs have not been
  tested on physical devices; Windows plugin generation requires Developer Mode.
- Chrome/Playwright ran mocked-API login, discovery, report, block/unblock,
  archive download and deletion flows at 390x844 and 1366x900. Both passed, with
  no runtime/overflow errors; screenshots and nonblank pixel checks were reviewed.
  These are UI tests, not live SMS or database erasure evidence.
- Backend CI now requires a healthy Docker engine and rejects skipped tests.
  The updated CI workflow has not been run on a remote runner in this session.
- AWS STS rejected the local `bharatchat-mumbai-prod` profile as expired. No
  EC2 instance or application deployment was created in this session.
- DNS resolution returned `ns45.domaincontrol.com` and `ns46.domaincontrol.com`
  as the authoritative nameservers for `bharatchat.in`. No DNS records, TLS
  certificates or MSG91 delivery have been changed or verified live.

Mobile export sharing writes an app-private temporary archive. Startup and
subsequent-export cleanup remove owned copies older than one hour, including
Android's share-plugin copies. User-selected saved/shared copies remain outside
the app's control. This is file deletion, not a guarantee of storage zeroization.
Encrypted-media object erasure, backup expiry, long-offline recovery, and
multi-device deletion convergence remain launch work.

Historical verification recorded on 2026-09-15/16 (not revalidated here):

- The new prelaunch config, OTP allowlist and message-lock unit tests pass, as
  does the complete Go suite after wiring those controls into server startup.
- Host provisioning tests use a fake AWS CLI to exercise account/network rejection,
  create failure, WhatIf, termination protection and instance/IP output recording.
  ShellCheck, rendered firewall validation, secret filtering, Nginx TLS syntax and
  CloudFormation lint pass. No real cloud bootstrap or firewall test is implied.

- The full Go suite passes, including real PostgreSQL/Redis integration tests,
  successful OTP login/rotation/revocation, and direct-chat block enforcement.
- `go vet ./...` and the server binary build pass.
- Flutter analysis has zero issues and all 32 Flutter tests pass. The loopback
  debug web bundle builds and is served by the new `web` Compose profile.
- A separate release web build succeeds with `https://api.bharatchat.in/api/v1`,
  no plaintext override, and locally bundled Flutter web assets instead of CDN
  loading. This verifies compilation, not completed encrypted messaging.
- The Nginx release image builds from that bundle. A temporary, read-only local
  container with restricted capabilities and a test certificate serves HTTPS 200,
  redirects HTTP with 308, and proxies an unauthenticated API request to the local
  development backend with 401. This is not an AWS or public-certificate test.
- Playwright runs the mocked-API login, username lookup, report, block, and unblock
  flow at 390x844 and 1366x900. Both pass with no page errors or detected Flutter
  layout overflows; screenshots and nonblank pixel checks were reviewed.
- AWS Compose schema validation, ShellCheck, secret-loader positive/negative
  tests, literal credential handling, and Nginx TLS syntax validation pass locally.
  These tests use dummy credentials and certificates, not AWS or MSG91 accounts.
- The prelaunch EC2 CloudFormation template passes local `cfn-lint` 1.56.3.
  It has not been submitted to AWS, and its host startup has not been tested there.
- The local backend was restarted with the current code and `/readyz` returns 200.

The historical Windows run did not enable the race detector because a suitable
Windows C toolchain was unavailable. The Linux container runner now provides
that toolchain; consult the current snapshot above for this revision's evidence.
The dependency scan and Docker build below are historical, not a new audit.

Earlier verification on 2026-09-14:

- Every Go package compiles; configuration, auth, message, and group service
  suites pass; the production server binary builds.
- `go mod verify`, `gofmt`, `go vet`, and `govulncheck` pass. The vulnerability
  scan reports zero reachable vulnerabilities.
- Flutter 3.47.2 reports zero analyzer issues, all 25 tests pass, and the
  JavaScript web release builds with an HTTPS API URL.
- Docker Compose configuration validates successfully.
- The complete Go suite passes against real Testcontainers-managed PostgreSQL and
  Redis instances, including migrations, auth, HTTP/WebSocket messaging, and Redis
  pub/sub fan-out integration tests.
- The production Docker target builds successfully. A container started from that
  exact image ran migrations against the Compose PostgreSQL service, connected to
  Redis, remained healthy, and returned 200 from both `/healthz` and `/readyz`.
- The loopback-only Compose PostgreSQL, Redis, and pinned MinIO community hotfix
  containers all report healthy.

Windows still needs Developer Mode for Flutter plugin symlinks, an Android SDK for
Android artifacts, and Visual Studio C++ tooling for Windows desktop artifacts.

## Blocking Public Launch

1. Integrate and independently review audited Signal Protocol or MLS client
   bindings on every shipped Flutter platform. The current app deliberately does
   not invent its own messaging protocol.
2. Provision Android and Apple release identities and test signed artifacts on
   physical devices. Platform runners are checked in, source analysis and all 45
   Flutter tests pass, and the JavaScript web release builds successfully; Android
   SDK, Apple signing, background delivery, and notification setup remain external
   release prerequisites.
3. Deploy and load-test the OTP SMS adapter, PostgreSQL, Redis, object storage,
   ingress, backups, restore drills, monitoring, alerting, and incident response.
4. Test multi-replica WebSocket fan-out, reconnect storms, failover, and rolling
   deploys in the selected production environment.
5. Complete abuse controls, moderation operations, account recovery, support,
   privacy/legal documents, data export/deletion verification, and app-store review.
6. Commission independent mobile, backend, cryptographic protocol, and cloud
   infrastructure security assessments; remediate findings before inviting users.

No marketing or UI should claim end-to-end encryption until gate 1 is complete
and the exact shipped binaries have passed external review.

The ordered engineering and operations backlog is in [LAUNCH_ROADMAP.md](LAUNCH_ROADMAP.md).
