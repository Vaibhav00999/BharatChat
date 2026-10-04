# AWS Mumbai Deployment

Target: `bharatchat.in` and `api.bharatchat.in`, AWS `ap-south-1`, Ubuntu EC2,
Docker, and Nginx. The dedicated deployer and restricted runtime IAM stack are
created and verified, but no EC2 host has been provisioned. DNS has not been
changed, and no live SMS has been sent. Applying its host template
creates billable resources but does not establish that the app is ready for users.
Public launch remains blocked by the client encryption and operational gates in
[`PRODUCTION_READINESS.md`](../../docs/PRODUCTION_READINESS.md).

Hosting was explicitly paused by the owner on 2026-10-02 after AWS rejected the
reviewed `t3.medium` because it was not Free Tier-eligible. The pilot stack reached
`ROLLBACK_COMPLETE`; both the attempted host and security group are
`DELETE_COMPLETE`. Actual Mumbai queries found no BharatChat instances or Elastic
IPs. The failed stack record remains termination-protected for inspection; the
successful IAM bootstrap remains intact. No billing-plan change, smaller-instance
substitution, stack deletion, or retry was performed. Evidence is in ignored
`runtime/host-attempt-20261002.json` and `runtime/host-failed-events-20261002.json`.
Resuming hosting requires the owner's decision on billing and inspected,
administrator-authorized handling of this exact rolled-back stack; a create-only
script cannot overwrite it. Public launch is still a separate, blocked milestone.

## Current Access And DNS

AWS CLI v2.36.46 is installed locally. On 2026-09-20, `bharatchat-mumbai-prod`
successfully switched from Root to the deployer after opening the approval in
Chrome, where the owner had signed in as the IAM user. `sts get-caller-identity`
confirmed the expected account's `bharatchat-deployer` IAM user. Account-specific
identifiers are kept in local runtime evidence, not this public runbook.
Root access is authorized only for the dedicated deployer's onboarding, not to
deploy the app. Future logins must again verify the returned identity.
Do not send credentials or authorization codes in chat.

On 2026-10-02, the renewed profile again passed STS as the same dedicated
deployer. Mumbai instance inventory was empty. Read-only host preflight passed
for the reviewed default VPC and public subnet (ap-south-1a),
and the current Ubuntu image `ami-007b1f3fdea0383d9`. EC2 confirmed that image is
available, x86-64, and owned by Canonical (`099720109477`). The pinned image in
both stack parameters must match; do not reuse September's stale policy document.
Reading the fixed runtime instance profile was initially denied; the successful
one-time IAM bootstrap below resolved that prerequisite. The host dry-run
alone does not establish write permissions or runtime profile existence. No stack,
EC2 instance, DNS record, certificate, or live SMS was created by these checks.

Console inspection on 2026-10-02 showed only `bharatchat-deployer` and the two
AWS service-linked roles, not the separate administrator initially assumed.
The owner authorized the separate `bharatchat-iam-bootstrap` Root login only
for this one-time IAM setup. Root must not create EC2 or deploy the application.
Keep application provisioning under `bharatchat-mumbai-prod`; the profile name
alone never proves which identity is active.
The sign-in helper now rejects Root, `bharatchat-deployer`, wrong accounts and
failed STS lookups. It verifies identity only, not administrator permissions, and
does not create resources or modify policies. On 2026-10-02, STS showed the old
`bharatchat-owner-bootstrap` profile was still the deployer; it must be renewed
using a separate administrator identity if one is established. That helper is
not the explicitly authorized one-time Root fallback described here.
The reusable helper requires an explicit expected account:
`./deploy/aws/login-owner.ps1 -ExpectedAccountId YOUR_ACCOUNT_ID`.

### First Live IAM Attempt

On 2026-10-02, STS verified the one-time Root session. Live checks confirmed
Root MFA, no Root access keys, and deployer MFA. Template lint, offline policy
checks and Access Analyzer passed, but the first actual IAM stack failed:
the provisioning policy exceeded the IAM user's 2,048-character aggregate inline
quota. The offline test had incorrectly used the 10,240-character role quota.
Access Analyzer/template validation do not check that deployment quota.

AWS completed rollback of this attempt; no EC2 host was created and the failed
attempt did not establish provisioning permissions. The failed stack ID/reason
are recorded in ignored `runtime/iam-attempt-20261002.json`.
The owner profile was logged out in the execution guard's `finally` block;
a subsequent STS request confirmed that its local login session was unavailable.
This does not revoke other already-loaded tokens or the owner's browser session.

The corrected template uses a customer managed policy with unchanged permission
statements, not a new administrator grant. For the selected live parameters it
is 5,124/6,144 characters; the onboarding inline template is 1,782/2,048.
Template lint, corrected quota regression checks, mocked provisioning/onboarding
and identity tests, and AWS template validation pass.

After the owner renewed the explicitly authorized IAM-only session on 2026-10-02,
the exact failed stack ID was inspected: all three resources were
`DELETE_COMPLETE` and the stack was `ROLLBACK_COMPLETE`. Its events were archived
in ignored `runtime/iam-failed-events-20261002.json`, then only that failed stack
record was removed. The corrected stack reached `CREATE_COMPLETE` with termination
protection enabled. Its outputs are recorded in ignored
`runtime/iam-completed-20261002.json`.

IAM simulation confirmed pilot-stack creation is allowed while user-policy
editing and direct secret reads are implicitly denied and access-key creation
is explicitly denied. Actual deployer STS identity and runtime instance-profile
reads succeeded. The one-time Root profile was logged out in `finally`; a
subsequent STS lookup confirmed its cached login was unavailable. Root was not
used to create EC2 resources. Host creation and boot-time verification remain
separate, pending work; IAM simulation is not an actual host deployment test.

The IAM user `bharatchat-deployer` was created under `/bharatchat/` on 2026-09-16.
It was initially created without a password or access keys. On 2026-09-20, the rendered onboarding
policy was attached as `BharatChatOnboarding`, along with AWS's
`SignInLocalDevelopmentAccess`. IAM Access Analyzer reported no findings for the
rendered policy, and IAM policy simulation confirmed the intended allow/deny rules.
The onboarding policy permits self-service password/MFA enrollment and limited
Mumbai preflight reads. It does not grant infrastructure-write or secret-read
permissions; those require a separately reviewed deployment policy after MFA setup.

On 2026-09-20, live IAM checks verified Root MFA enabled, no Root access keys,
a console login profile for `bharatchat-deployer` with no password reset required,
and one MFA device assigned to the deployer. The owner completed these steps
directly in AWS; passwords and MFA secrets were not handled by deployment tools.
For additional deployer virtual MFA devices, use the `bharatchat-deployer` prefix
to match the enrollment policy. Never create root or deployer API access keys.

The policy JSON is a template: run `render-onboarding-policy.ps1 -AccountId ...`
to generate the document for the verified account before validation or attachment.
IAM policy variables cannot replace the account field of a resource ARN. The
original dynamic-account document passed Access Analyzer but was rejected by
`PutUserPolicy`; the corrected document was accepted. Rendered account-specific
files belong in ignored `runtime/`, not the reusable policy template.

Historical onboarding-only decisions: Mumbai VPC reads and own-user password/MFA
enrollment are allowed; instance launch, stack creation, user-policy writes, and
Secrets Manager reads are denied. Access-key creation is explicitly denied.
The permission decisions above were checked using IAM simulation. Actual CLI
sign-in and `create-host.ps1 -WhatIf` have now also passed as the deployer: the
selected Mumbai subnet, VPC DNS support, active Internet Gateway route, and
CloudFormation template validation succeeded. `-WhatIf` did not create a stack
or validate infrastructure-write permissions. The separately reviewed provisioning
policy was subsequently attached by the successful 2026-10-02 IAM bootstrap.

On 2026-09-22, the deployer session was renewed and the provisioning policy was
prepared in `provisioning-iam.yml`. At that time it had not been applied. Both templates pass
local lint, the policy invariants pass, and AWS validates the IAM template. These
checks do not establish that every write operation will succeed. Owner approval
must use a separate `bharatchat-owner-bootstrap` profile. If automatic browser
selection reuses the deployer session, `login-owner.ps1` uses `aws login --remote`:
open the URL in Chrome Incognito and enter the resulting authorization code only
in the local terminal. Verify STS identity, never infer it from the profile name.

```powershell
aws login --profile bharatchat-mumbai-prod --region ap-south-1
aws sts get-caller-identity --profile bharatchat-mumbai-prod --region ap-south-1
```

For an organization using IAM Identity Center, use `aws configure sso` and
`aws sso login` with that profile instead. Never use the root account to deploy.

Public DNS checked on 2026-09-16 reports `ns45.domaincontrol.com` and
`ns46.domaincontrol.com`, consistent with GoDaddy DNS, not Route 53. The apex
currently resolves to `3.33.130.190` and `15.197.148.33`; `api.bharatchat.in` did not
resolve. These observations do not prove registrar/account ownership. Confirm
access to the authoritative zone and export its current records before editing.
Do not replace nameservers, MX/TXT records, or the existing apex service blindly.
Once an actual Elastic IP exists, plan an API A record and an explicitly reviewed
apex cutover. DNS changes have not been made.

## Controlled Pilot Gates

- `OTP_ALLOWED_PHONES` is required outside development: 1-20 unique, canonical
  E.164 numbers separated by commas, without spaces. Store it in Secrets Manager
  alongside MSG91 credentials. Only owned or consenting tester numbers belong here.
- Both OTP request and verification check the allowlist before creating a challenge,
  sending SMS, or establishing a session. Removing a number does **not** revoke
  previously issued sessions; use explicit session revocation as well.
- `MESSAGING_ENABLED=false` locks the backend send path for both plaintext and
  ciphertext. Configuration refuses to enable it outside development. Compose
  fixes it to false, and the secret loader rejects any attempt to override it.
- Unlocking messaging requires a reviewed code/config release after the actual
  client E2EE implementation and independent review. A protocol label on an opaque
  payload is not evidence of encryption. This deployment can test infrastructure
  and allowlisted login only; it is not yet a user messaging pilot.

## Infrastructure Prerequisites

No EC2 instance exists yet (verified as the deployer using the Mumbai EC2 API on 2026-09-20).
Read-only checks found the default VPC and three available public subnets with an
active Internet Gateway route. No IAM Identity Center instance was found in Mumbai.
`ec2-host.yml` prepares the host, Elastic IP, and restricted security group in an
**existing** Mumbai VPC/public subnet, using a separately pre-created instance
profile. It requires a tester's
single IPv4 `/32`; it does not open the service to everyone or add an SSH key.
It requires IMDSv2 with hop limit 1 and an encrypted root volume. It does not
create RDS, Redis, DNS, certificates, ECR repositories, or application secrets,
and it does not install Docker or deploy code. Follow the remaining steps below.

### One-Time IAM Bootstrap

An authorized administrator must first review and create the
`bharatchat-provisioning-iam` stack from `provisioning-iam.yml`. This creates the
fixed runtime role/profile and attaches `BharatChatPrelaunchProvisioning` to the
existing deployer, without giving that user IAM editing permissions. Pin the same
reviewed Canonical Ubuntu image in both stacks. Resolve the public SSM parameter
with the deployer, then verify the AMI owner is Canonical (`099720109477`), the
image is available, and its architecture is x86-64. The image pinned in the
successful 2026-10-02 bootstrap is `ami-007b1f3fdea0383d9` in Mumbai; do not reuse
the older September image.

The policy allows creating only `bharatchat-prelaunch`, without a CloudFormation
service role. EC2 writes require CloudFormation in `aws:CalledVia` and are limited
to Mumbai, the reviewed network/image and tagged pilot resources. Instance size,
encrypted volume size/type, the runtime profile, and IMDSv2 are constrained.
`iam:PassRole` allows only the fixed runtime role to EC2. It grants no IAM editing,
DNS, database provisioning, or direct Secrets Manager access. SSM shell commands
are limited to tagged pilot instances; host administrators can access the host's
runtime credentials and application secrets, so this remains a trusted operator
identity. It is not a general-purpose administrator account.

From the repo root, using the separately authenticated owner profile:

```powershell
aws sts get-caller-identity --profile bharatchat-owner-bootstrap
aws cloudformation create-stack --profile bharatchat-owner-bootstrap --region ap-south-1 `
  --stack-name bharatchat-provisioning-iam --enable-termination-protection `
  --template-body file://deploy/aws/provisioning-iam.yml --capabilities CAPABILITY_NAMED_IAM `
  --parameters ParameterKey=VpcId,ParameterValue=vpc-REPLACE `
    ParameterKey=PublicSubnetId,ParameterValue=subnet-REPLACE `
    ParameterKey=UbuntuImage,ParameterValue=ami-REPLACE
aws cloudformation wait stack-create-complete --profile bharatchat-owner-bootstrap `
  --region ap-south-1 --stack-name bharatchat-provisioning-iam
aws logout --profile bharatchat-owner-bootstrap
```

Do not proceed if creation failed. Inspect events, preserve evidence and do not
delete/retry blindly. The deployer cannot modify or delete this IAM stack.

### Pilot Host

The template passes local `cfn-lint` 1.56.3 and AWS `validate-template` validation.
Preflight confirmed the default subnet routing, but deployment permissions, quotas,
write authorization and boot-time behavior still need verification. Template validation
does not create any resources or prove a successful deployment.

`create-host.ps1` automates account/subnet/route checks, create-only provisioning,
termination protection, waiting for completion, and recording the real instance
ID/IP in ignored `deploy/aws/runtime/host.json`. Submitted stack IDs are also
recorded immediately in separate attempt files, and verification failures preserve
events when readable. Completed status, termination protection and all required
outputs are checked before a successful host record is written. It never edits
DNS or automatically retries/deletes a failed stack. An existing
stack is not updated or deleted. Read-only preflight also runs with `-WhatIf`:

```powershell
./deploy/aws/create-host.ps1 -ExpectedAccountId YOUR_ACCOUNT_ID `
  -VpcId vpc-REPLACE -PublicSubnetId subnet-REPLACE -TesterIPv4 YOUR_PUBLIC_IP/32 `
  -UbuntuImage ami-REPLACE -WhatIf
# Remove -WhatIf only after reviewing the account, cost, network, and template.
```

The script's mocked tests cover Root/wrong-account rejection, network guards,
failed waits, unreadable events, incomplete/unprotected stacks, missing outputs,
and dry-run behavior. The 2026-10-02 live attempt verified IAM acceptance for
network creation but failed host launch on the account's Free Tier restriction;
there is no successful host execution test. It requires an existing VPC/public
subnet; it intentionally does not guess a network or create a new VPC.

Creating the host incurs EC2, EBS, and public IPv4 costs. Review the template,
budget, selected AMI, networking, and IAM policy before executing it in AWS. No
change set has been submitted by this workspace. Enable stack termination
protection after creation. Stack deletion terminates the host and its root volume;
the live database must never be stored there. Use DNS-based certificate validation
while HTTP is restricted to the tester. The host role scopes secret reads to the
exact `bharatchat/production` secret name plus AWS's six-character ARN suffix.

1. Use an Ubuntu x86-64 EC2 instance with encrypted EBS, IMDSv2 required, and an
   instance profile. Manage it through Systems Manager Session Manager, not a
   publicly accessible SSH port. Restrict 80/443 to the release team's IPs during
   prelaunch testing. Only open them publicly after release approval.
2. Use private RDS PostgreSQL 16, encryption at rest, automated backups, deletion
   protection, and Multi-AZ for production. Its security group allows 5432 only
   from the application's EC2 security group. Require TLS and install the AWS RDS
   CA bundle at `/etc/bharatchat/rds-ca.pem` (root-owned, mode 0644). Use a dedicated
   database and database owner for the current startup migrations, not the RDS
   master account. A separately scoped migration job remains a hardening task.
3. Use a private, cluster-mode-disabled ElastiCache Redis OSS replication group
   with encryption in transit/at rest, an AUTH token, and automatic failover.
   Allow 6379 only from EC2. Use the primary endpoint as both `REDIS_HOST` and
   `REDIS_TLS_SERVER_NAME`. The current client does not support Redis Cluster
   routing or IAM token refresh. Test AUTH token rotation before launch.
4. Install Docker Engine, Compose >= 2.30, AWS CLI v2, `jq`, `curl`, `openssl`, and
   `flock` on Ubuntu using the suppliers' installation instructions. Keep security
   updates and Docker log rotation enabled. Do not run the repository's dev Compose
   file on this host. There are no database ports published by this production file.
   For a fresh Ubuntu 24.04 amd64 host, transfer this directory and run
   `sudo bash /opt/bharatchat/deploy/aws/bootstrap-host.sh YOUR_PUBLIC_IP/32` through
   Session Manager. It refuses an existing Docker installation or active UFW
   configuration. It installs official Docker apt packages and the AWS-published
   CLI snap, UFW, unattended updates, and a Docker `ExecStartPost` firewall hook.
   No SSH port is opened. Docker's iptables backend is required: the dedicated
   `BHARATCHAT-PILOT` chain restricts forwarded container ingress to tester HTTP/S;
   UFW alone does not protect Docker-published ports. The AWS security group must
   retain the same restrictions. IPv6 ingress is not enabled by this template.
   Recheck firewall behavior from a second, non-allowlisted network and after
   host/Docker restarts before deployment. Local tests only render these rules;
   bootstrapping has not yet run on a real EC2 host.
5. Point both DNS names to the EC2 Elastic IP. Obtain a trusted certificate with
   both names using your certificate authority. Install it at
   `/etc/letsencrypt/live/bharatchat.in/{fullchain.pem,privkey.pem}` and mount the
   entire `/etc/letsencrypt` tree so renewal symlinks continue to work. Keep the
   private key root-only. Configure automatic renewal and Nginx reload; alert
   before expiry. This bundle does not automate certificate issuance.

The application uses `172.30.91.0/24` for its Docker network and trusts only the
Nginx container's `.2` address. Check for VPC/VPN/host-network overlap before using
it. Change both the network and `TRUSTED_PROXIES` together if necessary. Do not put
a load balancer/CDN in front without updating and testing trusted-proxy handling.

## Secret And Instance Role

Create one Secrets Manager **JSON object** in Mumbai. All values must be strings.
Required keys are `JWT_ACCESS_SECRET`, `POSTGRES_HOST`, `POSTGRES_USER`,
`POSTGRES_PASSWORD`, `POSTGRES_DB`, `REDIS_HOST`, `REDIS_PASSWORD`,
`REDIS_TLS_SERVER_NAME`, `OTP_MSG91_AUTH_KEY`, `OTP_MSG91_TEMPLATE_ID`, and
`OTP_ALLOWED_PHONES`.
Generate the JWT secret from at least 32 random bytes. See `secret-env.jq` for the
small optional settings allowlist. Do not add `APP_ENV` or plaintext overrides:
Compose enforces the production settings independently of the secret.

Use an approved MSG91 OTP template containing `##OTP##`, enable the correct
delivery route, and verify your account's DLT/provider requirements with MSG91.
Live delivery has not been tested. See [`OTP_DELIVERY.md`](../../docs/OTP_DELIVERY.md).

Adapt `instance-policy.example.json` with the real account, exact secret ARN, and
two ECR repositories. Attach it to the EC2 role, along with the Session Manager
permissions required by your organization. A customer-managed secret KMS key also
needs scoped `kms:Decrypt` permission and a matching key policy. Do not place AWS
access keys in the application container or grant wildcard secret access.

The deploy script fetches secrets using the host's AWS credential chain (use the
instance role), writes a mode-0600 raw env file under `/run`, and removes it on exit.
It never sources that file or prints its contents. Raw mode preserves literal `$`
and quotes. Docker retains container environment values in its metadata: root and
Docker administrators can read them. Encrypted EBS and restricted operator access
are still necessary. Secret rotation requires rerunning deployment to recreate the
backend; simply restarting it does not fetch a new secret.

## Build And Deploy

Run the Go, Flutter, dependency-security, and image scans before publishing images.
Build on a trusted CI runner, not on the EC2 production machine. From the repo root:

```bash
cd frontend
flutter pub get
flutter build web --release --no-web-resources-cdn \
  --dart-define=BASE_URL=https://api.bharatchat.in/api/v1
cd ..
docker build --platform linux/amd64 --target production -t bharatchat-backend:release backend
docker build --platform linux/amd64 -f deploy/aws/edge.Dockerfile -t bharatchat-edge:release .
```

Never enable `ALLOW_PLAINTEXT_MESSAGING` for a release. The release currently blocks
message composition because audited client protocol integration is unfinished.
Package signing, SBOMs, and an image admission policy must be added before launch.
Push reviewed images to ECR in Mumbai, record their digests and source revision,
and transfer this deployment directory to a root-controlled path on EC2.

```bash
sudo bash /opt/bharatchat/deploy/aws/deploy.sh \
  arn:aws:secretsmanager:ap-south-1:ACCOUNT_ID:secret:SECRET_NAME-SUFFIX \
  ACCOUNT_ID.dkr.ecr.ap-south-1.amazonaws.com/bharatchat-backend@sha256:BACKEND_DIGEST \
  ACCOUNT_ID.dkr.ecr.ap-south-1.amazonaws.com/bharatchat-edge@sha256:EDGE_DIGEST
```

This command validates secrets/certificate/config, pulls digest-pinned images,
checks Nginx syntax, waits for PostgreSQL/Redis readiness, and checks the HTTPS web
root. It does not perform a paid SMS test. Use an owned handset for the separate
OTP login, resend, expiry, and replay checks. Never send raw OTPs to application
logs or telemetry. Keep request URL capture disabled for MSG91 and authentication
header capture disabled for WebSocket tickets.

Containers restart after host reboot; deployment staging secrets are intentionally
not persisted in `/run`. To reload Nginx after certificate renewal, select its
Compose container by the `com.docker.compose.project=bharatchat-production` and
`com.docker.compose.service=edge` labels and run `nginx -t` then `nginx -s reload`.

## Operations And Limits

- This is **one EC2 host**, not a highly available application tier. RDS and Redis
  redundancy do not remove that single point of failure. Plan ALB/ASG or another
  orchestrated multi-instance topology and test WebSocket failover before claiming HA.
- Startup migrations run before the backend becomes ready. Take and test backups
  first. A failed rollout is reported as failure and does not automatically undo
  schema changes. Restore a reviewed previous image only when schema-compatible;
  otherwise apply a forward fix. Never run destructive down migrations on live data.
- Set CloudWatch alarms for host/container health, disk, resource saturation,
  database/cache failures, certificate expiry, and SMS error rate/cost. Establish
  on-call coverage, retention limits, restore drills, and an incident runbook.
- Set AWS budget alerts and MSG91 spend/rate limits. No AWS resources or billable
  provider operations are created by the local configuration checks.
- Block public signup until encrypted messaging, moderation handling, deletion/
  export checks, signed platform builds, and independent security review pass.

## Reference Documentation

- [Docker Compose env files and raw format](https://docs.docker.com/reference/compose-file/services/#env_file)
- [Nginx WebSocket proxying](https://nginx.org/en/docs/http/websocket.html)
- [AWS RDS TLS and CA certificates](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html)
- [Secrets Manager role policies](https://docs.aws.amazon.com/secretsmanager/latest/userguide/auth-and-access_iam-policies.html)
- [ElastiCache AUTH requirements](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/auth.html)
- [Canonical Ubuntu AMI discovery](https://ubuntu.com/aws/docs/aws-how-to/instances/find-ubuntu-images/)
- [EC2 metadata options](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-properties-ec2-instance-metadataoptions.html)
- [AWS CLI browser authentication](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sign-in.html)
- [GoDaddy authoritative DNS](https://developer.godaddy.com/en/docs/api-users/troubleshoot/dns)
- [Docker Ubuntu installation and firewall limitations](https://docs.docker.com/engine/install/ubuntu/)
