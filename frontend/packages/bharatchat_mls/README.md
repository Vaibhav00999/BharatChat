# Native MLS Component

Experimental, prelaunch integration. This package is **not imported by the
shipping Flutter app** and does not unlock messaging or establish launch readiness.
It uses real OpenMLS cryptography through the community `openmls` Dart package,
pinned to 3.2.1, rather than implementing a new ratchet or encryption protocol.

## Current Scope

- Native MLS suite 0x0001: X25519, AES-128-GCM, SHA-256, Ed25519. Experimental
  post-quantum suites are not enabled.
- Single-use, seven-day KeyPackages, encrypted handshakes, group creation,
  Welcome processing, application encryption/decryption, removal and epoch update.
- SQLCipher state with a random 32-byte key. The database key and serialized
  signing identity must be stored together by a platform `MlsSecretStore` adapter,
  never preferences, source files or the database itself.
- Account/device-scoped files, an exclusive file lock, initialization recovery
  without changing identity, and failure when initialized state or secrets vanish.
- No retained past epochs or resumption PSKs. This limits old-key retention but
  sacrifices late messages from prior epochs; it is not an offline recovery design.

The caller supplies an app-private directory and authenticated account/device
UUIDs. The package does not implement OS directory permissions or secure storage.
Tests use a memory secret store only as a fixture; cryptographic operations and
encrypted database persistence use the actual native library.

## Required Before App Integration

Basic credentials contain self-asserted account/device identifiers. They are not
proof of identity. Before calling `addMembers`, the application must authenticate
and pin each KeyPackage's identity/key binding, with key-change verification UI.
Before processing a commit, the integration must enforce its membership policy.
The current wrapper automatically applies cryptographically valid commits; it
does not enforce BharatChat's owner/admin permissions.

The existing backend Ed25519/X25519 prekey directory is not an MLS KeyPackage
directory. Device registration, one-time package claiming, device-targeted
Welcome delivery, ordered commits, epoch conflict handling, and crash-safe
transport/outbox transactions still need implementation and review. Creating a
commit advances local state before delivery, so a lost commit must not be treated
as a successfully synchronized group.

Encrypted local plaintext history and a sender's own-message copy are not built
here. Device-loss recovery, backup consent, revocation convergence, and account
deletion need explicit application policies. Do not silently replace missing
state or restore stale ratchet state. Exclude database and secret material from
automatic OS backups until that design is reviewed.

The component is native-only (`dart:io`), not a browser implementation. Windows
native tests passed on 2026-10-02; Android/iOS physical-device behavior and other
platforms have not been verified locally. File locks have not been verified
across processes/isolates. Mutable private buffers are wiped on close/failure,
but Dart strings and FFI copies prevent a guarantee of memory zeroization.

An upstream protocol/library audit does not audit this community Dart wrapper,
its downloaded native artifacts, or BharatChat's integration. Review dependency
provenance, reproducible native builds, storage, transport, and the complete
protocol glue independently before enabling messaging. No claim of superior
privacy to another messenger follows from these tests.

## Verification

With a compatible Dart SDK (>=3.10), from this directory:

```bash
dart pub get --enforce-lockfile
dart format --output=none --set-exit-if-changed lib test
dart analyze --fatal-infos
dart test --concurrency=1 --reporter=expanded
```

Native build hooks download the pinned dependency's platform artifact. Fourteen
tests cover genuine two-way encryption, tamper/replay rejection, three-member
removal, epoch changes, encrypted persistence/reopen, wrong database keys,
missing state/secrets, account isolation, duplicate handles, interrupted
initialization, Welcome binding/reuse, UUID validation and closed sessions.
Expected SQLCipher HMAC errors during the wrong-key test are not test failures.

References: [OpenMLS](https://github.com/openmls/openmls),
[Dart wrapper](https://github.com/djx-y-z/openmls_dart),
[pinned release](https://pub.dev/packages/openmls/versions/3.2.1).
