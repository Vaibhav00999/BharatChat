# BharatChat Privacy Architecture

## Objective

BharatChat is designed so the service can route messages without learning their
content. Privacy is a technical invariant, not a claim based on policy alone.

## Implemented Guarantees

- Production rejects newly submitted plaintext messages.
- The message API accepts only an explicit, versioned Signal-or-MLS ciphertext
  envelope in production; cryptographic validity remains the client protocol's job.
- Private identity keys and prekeys remain in client secure storage.
- The server verifies every signed prekey before publishing it.
- One-time prekeys are claimed atomically, retries from one installation are
  idempotent, and consumed key IDs are never republished. Claim attribution is
  erased after seven days while a non-identifying consumed-key tombstone remains.
- Key bundles are visible only to users sharing an active conversation.
- Random per-account installation IDs preserve device/key continuity without
  collecting hardware serials or advertising identifiers.
- WebSockets use 30-second, single-use tickets in the subprotocol header; neither
  tickets nor JWTs appear in browser history or proxy URL logs.
- Active WebSockets revalidate their session before dispatching an inbound event
  and before writing a queued outbound event, with a bounded database-check
  context. An idle connection is also checked every 15 seconds. A failed check
  closes the socket instead of delivering its queued events. An event already
  in flight when revocation commits cannot be recalled.
- New accounts hide phone discovery and last-seen, disable read receipts, and
  restrict group invitations to contacts.
- Group-add privacy honors blocks in both directions.
- Direct-chat creation and sending honor blocks in both directions.
- Avatar and online visibility are enforced in chat and group SQL read models.
- Disabled read receipts update local unread state without notifying senders.
- Typing indicators are opt-in and enforced by the server, not client convention.
- Group invite secrets are visible only to owners and admins.
- New chats use seven-day disappearing messages by default.
- Expired messages, OTP challenges, and old sessions are removed by a bounded
  background retention worker; old prekey claim attribution is anonymized.
- Request logs omit client IP addresses, query strings, message bodies, and keys.
- Production requires verified PostgreSQL TLS, Redis TLS, explicit CORS origins,
  authenticated datastore access, E2EE mode, HTTPS OTP delivery, security response
  headers, and configured trusted proxies.
- Browser messaging fails closed until non-exportable browser key storage exists.
- Pending outbound messages are stored in an AES-encrypted Hive box whose key is
  kept in platform secure storage. Records are scoped by authenticated user,
  retained until the server acknowledges the idempotency key, resent after
  reconnect/process restart, and erased with other local session data on logout.
- Reconnect refreshes the chat list from PostgreSQL-backed REST state, so Redis
  pub/sub remains a low-latency signal rather than a message recovery mechanism.

## Cryptographic Protocol Boundary

The opaque message envelope can carry versioned protocol payloads, but the current
Ed25519/X25519 prekey directory is preparatory infrastructure, not a compatible
Signal or MLS key format. It must be replaced or adapted to the selected protocol.
The final client cipher must be backed by an independently audited implementation
of the Signal Protocol or MLS, including asynchronous session setup, forward
secrecy, post-compromise security, multi-device fan-out, group sender-key rotation,
and safety-number verification.

Do not replace that bridge with a custom sequence of X25519 and AEAD calls. Sound
primitives are not, by themselves, a sound messaging protocol.

An isolated [native MLS component](../frontend/packages/bharatchat_mls/README.md)
now exercises real OpenMLS encryption, SQLCipher state, replay rejection and
membership epoch changes. It is not imported by the shipping app. Its basic
credentials are self-asserted; identity pinning, membership authorization,
KeyPackage registration, ordered commit/Welcome transport, recovery and an
independent integration review remain required. Upstream library audit coverage
must not be represented as an audit of the Dart wrapper or BharatChat.

Until the audited native bridge is integrated and externally reviewed,
`REQUIRE_E2EE=true` intentionally prevents the current plaintext composer from
sending. The Flutter composer is disabled by default and release builds reject
insecure API URLs or `ALLOW_PLAINTEXT_MESSAGING=true`. Local UI development may
explicitly enable that client flag while running a backend with
`REQUIRE_E2EE=false`.

## Metadata Still Visible To The Service

The service currently knows account phone numbers, chat membership, sender and
recipient identifiers, device identifiers, message timestamps, delivery state,
and approximate ciphertext size. Hiding this requires sealed-sender delivery,
private contact discovery, padding, batching, and a separately reviewed abuse
prevention design.

## Required Before Public Launch

1. Integrate audited libsignal or MLS bindings on every supported native platform.
2. Add recipient-device ciphertext envelopes and device-targeted delivery for
   multi-device Signal sessions, or replace the directory with reviewed MLS
   KeyPackages if MLS is selected.
3. Add safety-number and key-change verification UI.
4. Rotate group sender keys on every membership change.
5. Add encrypted local message storage and encrypted, user-controlled backups.
6. Complete private contact discovery without uploading raw address books.
7. Add sealed-sender routing and fixed-size padding where practical.
8. Commission independent mobile, backend, protocol, and infrastructure audits.
9. Publish the client, protocol glue, reproducible builds, privacy policy, and
   transparency reports for public scrutiny.
