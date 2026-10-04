# BharatChat

BharatChat is a cross-platform Flutter chat client backed by a Go/Gin API,
PostgreSQL, Redis pub/sub, and an S3-compatible local MinIO service. The current
implementation covers schema foundations, phone/OTP authentication, profiles,
real-time direct messaging, presence/typing events, and group chat management.

**Status: engineering preview, not approved for public messaging.** Publishing
this source does not deploy a service or complete the encryption/security gates.
The release messaging lock stays enabled. Hosting is paused by the owner.
The public source is at [Vaibhav00999/BharatChat](https://github.com/Vaibhav00999/BharatChat).
Remaining public-launch acceptance criteria are tracked in
[issue #10](https://github.com/Vaibhav00999/BharatChat/issues/10).

The privacy foundation adds per-device signed key bundles, one-time prekeys,
single-use WebSocket tickets, linked-device revocation, privacy-first account
defaults, and disappearing-message retention. See [docs/PRIVACY.md](docs/PRIVACY.md)
for exact guarantees and the audited client-protocol work still required.
The operational launch gates and required production configuration are tracked in
[docs/PRODUCTION_READINESS.md](docs/PRODUCTION_READINESS.md).
The prioritized next steps are in [docs/LAUNCH_ROADMAP.md](docs/LAUNCH_ROADMAP.md).
An isolated [native MLS component](frontend/packages/bharatchat_mls/README.md)
exercises genuine encryption and encrypted state, but is not wired into the app
and has not completed independent review. Public messaging remains locked.

## Architecture

- `backend/cmd/server`: composition root and graceful HTTP shutdown.
- `backend/internal/features`: feature-first domain, repository, service, and
  transport packages for auth, users, chats, messages, groups, and presence.
- `backend/internal/platform`: PostgreSQL, Redis, logging, validation, and the
  multi-instance WebSocket fan-out hub.
- `backend/migrations`: twelve ordered, reversible PostgreSQL migrations.
- `frontend/lib/core`: shared networking, routing, encrypted token storage,
  local caching, and theming.
- `frontend/lib/features`: Riverpod-powered auth, profile, chat list, messaging,
  and group workflows that mirror the backend feature boundaries.

REST owns authentication, chat lists, history, group administration, and bulk
read actions. WebSocket events own low-latency sends, delivery/read receipts,
typing state, and live updates. Server events always pass through Redis pub/sub,
so the same delivery path works with one backend instance or many.

## Local Backend

The local `backend/.env` contains development-only values and is ignored by Git.
Never reuse those credentials in a deployed environment.
For a fresh clone, initialize local configuration from `backend/.env.example`
before starting Compose; real environment files are deliberately not published.

```bash
cp backend/.env.example backend/.env
docker compose up -d postgres redis minio
cd backend
go mod tidy
go run ./cmd/server
```

The server applies migrations automatically and listens on
`http://localhost:8080`. In development, requested OTP codes are printed to the
server console. The selected production provider is `OTP_DELIVERY_MODE=msg91`.
The backend also supports `OTP_DELIVERY_MODE=webhook` for a
deployment-owned HTTPS adapter or `OTP_DELIVERY_MODE=twilio` for direct Twilio
Messaging with API-key credentials. See [docs/OTP_DELIVERY.md](docs/OTP_DELIVERY.md)
for provider and India DLT setup.

To run the live-reload backend in Compose, use `docker compose up -d --build
backend`. The host port can be changed without altering the container or API
contract, for example `$env:BACKEND_HOST_PORT='18080'; docker compose up -d
backend` in PowerShell when Windows has reserved port 8080.

Useful endpoints:

- `GET /healthz`
- `GET /readyz` (PostgreSQL and Redis readiness)
- `POST /api/v1/auth/otp/request`
- `POST /api/v1/auth/otp/verify`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/chats`
- `POST /api/v1/users/resolve` (exact username, authenticated)
- `GET /api/v1/users/me/blocked` (paginated)
- `PUT /api/v1/users/me/blocked/:userId`
- `DELETE /api/v1/users/me/blocked/:userId`
- `POST /api/v1/users/me/reports`
- `GET /api/v1/users/me/export` (bounded, private account archive)
- `DELETE /api/v1/users/me` (requires DELETE confirmation and current refresh token)
- `GET /api/v1/chats/:chatId/messages`
- `POST /api/v1/groups`
- `GET /api/v1/groups/:chatId`
- `POST /api/v1/auth/ws-ticket` (authenticated, returns a 30-second one-use ticket)
- `GET /api/v1/ws` (WebSocket upgrade; ticket sent as `ticket.<value>` subprotocol)

## Flutter Client

The repository includes platform runners for Android, iOS, web, Windows,
macOS, and Linux. Flutter 3.47.5 is the verified toolchain for this revision:

```bash
cd frontend
flutter pub get
flutter run \
  --dart-define=BASE_URL=http://10.0.2.2:8080/api/v1 \
  --dart-define=ALLOW_PLAINTEXT_MESSAGING=true
```

Use `http://localhost:8080/api/v1` for desktop or web when the backend runs on
the same machine. The plaintext flag is accepted only for local development;
release builds require HTTPS and refuse to start when that flag is enabled.

For the loopback browser preview on this machine, build with
`BASE_URL=http://127.0.0.1:18080/api/v1` and the development-only plaintext flag:

```bash
cd frontend
flutter build web --debug --no-wasm-dry-run --dart-define=BASE_URL=http://127.0.0.1:18080/api/v1 --dart-define=ALLOW_PLAINTEXT_MESSAGING=true
cd ..
docker compose --profile web up -d --no-deps frontend
```

Open `http://127.0.0.1:18081`. The `web` Compose profile serves the existing build
and does not rebuild Flutter automatically. Backend port 18080 must already be
configured as described above. This preview is not a public deployment.

`frontend/tool/smoke_web.cjs` exercises login, lookup, report, block/unblock,
account export, and deletion at phone and desktop sizes using Playwright, pngjs,
and JSZip. It requires a loopback preview and an installed Chrome browser; all
API calls are mocked and no SMS is sent. Set `QA_BASE_URL` to the preview URL.

## AWS Deployment

The selected target is Ubuntu EC2 in AWS Mumbai (`ap-south-1`) with Docker and
Nginx at `bharatchat.in`. The [deployment bundle](deploy/aws/README.md) includes
production Compose, HTTPS/WebSocket ingress, instance-role secret loading,
digest-pinned ECR deployment, configuration tests, and an operations runbook.
The restricted deployer/runtime IAM setup succeeded. The attempted pilot host
was rejected by AWS's Free Tier restriction and fully rolled back; hosting was
then explicitly paused. No EC2 host or public application is running. CLI login
sessions must be renewed and identity-checked when deployment is resumed. Read
the latest verification snapshot before provisioning.
MSG91 credentials, DNS/certificates, and the client encryption launch gate remain
required. Do not use the local development stack as production infrastructure.

## Verification

```bash
cd backend
go test ./internal/config ./internal/features/auth/service \
  ./internal/features/message/service ./internal/features/group/service -cover
go test ./... -run '^$'
go vet ./...
go build -buildvcs=false ./cmd/server

cd ../frontend
flutter analyze
flutter test
```

The migration, Redis hub, auth transport, and message transport integration
tests use Testcontainers and therefore require a running Docker engine.

On Windows, run `./backend/scripts/verify-container.ps1` from the repository
root in PowerShell 7.2 or newer, with Docker on PATH. It builds a Linux test
image with the C toolchain,
runs the complete suite with the race detector, rejects skipped tests, builds
the production image, and runs its isolated startup smoke test. Full test
evidence is saved in the ignored
`backend/test-results-container.jsonl`. The runner gives trusted repository
tests access to the local Docker socket; never run it against a production
engine or untrusted code. Its named volume stores only Go build-cache artifacts.
The smoke test uses fresh PostgreSQL/Redis containers, an internal network,
in-container loopback HTTP with no published ports, and dummy credentials. It
runs the candidate as a non-root,
read-only container in development mode with messaging disabled, verifies
migrations/readiness and unauthenticated rejection, and removes its test
containers and volumes. It does not validate production TLS or live SMS.
