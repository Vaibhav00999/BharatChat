# Security

BharatChat is an engineering preview. Do not use it for sensitive conversations
or deploy it as a public messaging service. Client encryption is experimental
and not connected to the shipped application; independent security review is
still required. Release messaging remains locked.

## Reporting A Vulnerability

Private vulnerability reporting is enabled on
[Vaibhav00999/BharatChat](https://github.com/Vaibhav00999/BharatChat/security).
Use its Security tab to report privately. If it is unavailable, open an issue asking
the maintainer for a private reporting channel, without exploit details,
credentials, message contents, phone numbers, or other personal information.
No security response time or staffed incident service is promised yet.

Do not publish real credentials or user data in issues, discussions, or pull
requests. Rotate exposed credentials at their issuing provider immediately;
deleting a file or commit does not revoke them.

## Development And Deployment

- Use disposable local data and credentials for development and tests.
- Keep environment files, signing keys, cloud credentials, runtime deployment
  evidence, and native encryption databases out of Git.
- Never disable the production messaging lock to bypass release gates.
- Treat access to a host's Docker socket or SSM shell as privileged.
- Do not make unreviewed changes to encryption, key recovery, or identity binding.

See [privacy guarantees and limitations](docs/PRIVACY.md),
[production readiness](docs/PRODUCTION_READINESS.md), and
[launch roadmap](docs/LAUNCH_ROADMAP.md).
