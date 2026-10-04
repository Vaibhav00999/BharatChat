#!/usr/bin/env bash
set +x
set -euo pipefail
umask 077

if [[ $EUID -ne 0 || $# -ne 3 ]]; then
  printf 'Usage (on EC2): sudo bash deploy.sh SECRET_ARN BACKEND_IMAGE@sha256:DIGEST EDGE_IMAGE@sha256:DIGEST\n' >&2
  exit 2
fi
for dependency in aws jq docker curl openssl flock; do
  command -v "$dependency" >/dev/null || { printf 'Missing dependency: %s\n' "$dependency" >&2; exit 2; }
done

export AWS_REGION=ap-south-1 AWS_DEFAULT_REGION=ap-south-1 AWS_PAGER=''
secret_arn=$1
export BACKEND_IMAGE=$2 EDGE_IMAGE=$3
image_pattern='^[0-9]{12}\.dkr\.ecr\.ap-south-1\.amazonaws\.com/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$'
[[ $secret_arn =~ ^arn:aws:secretsmanager:ap-south-1:[0-9]{12}:secret: ]] || { echo 'Expected a Mumbai secret ARN' >&2; exit 2; }
[[ $BACKEND_IMAGE =~ $image_pattern && $EDGE_IMAGE =~ $image_pattern ]] || { echo 'Use Mumbai ECR image digests, not mutable tags' >&2; exit 2; }
[[ -r /etc/bharatchat/rds-ca.pem ]] || { echo 'Install the RDS CA bundle first' >&2; exit 2; }
cert=/etc/letsencrypt/live/bharatchat.in/fullchain.pem
[[ -r $cert && -r /etc/letsencrypt/live/bharatchat.in/privkey.pem ]] || { echo 'Install the domain certificate and key first' >&2; exit 2; }
openssl x509 -in "$cert" -noout -checkend 604800 >/dev/null
for hostname in bharatchat.in api.bharatchat.in; do
  openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt -untrusted "$cert" \
    -verify_hostname "$hostname" "$cert" >/dev/null
done

directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
install -d -m 700 /run/bharatchat
exec 9>/run/bharatchat/deploy.lock
flock -n 9 || { echo 'Another deployment is in progress' >&2; exit 2; }
runtime=$(mktemp -d /run/bharatchat/deploy.XXXXXX)
cleanup() {
  rm -f -- "$runtime/backend.env" "$runtime/docker/config.json"
  rmdir -- "$runtime/docker" "$runtime" 2>/dev/null || true
}
trap cleanup EXIT
export BACKEND_ENV_FILE="$runtime/backend.env" DOCKER_CONFIG="$runtime/docker"
install -d -m 700 "$DOCKER_CONFIG"

# Pipe the JSON directly into validation. Neither AWS responses nor env values are logged.
aws secretsmanager get-secret-value --secret-id "$secret_arn" --query SecretString --output text \
  | jq -er -f "$directory/secret-env.jq" > "$BACKEND_ENV_FILE"

compose=(docker compose -f "$directory/compose.yml")
# Compose >= 2.30 is required for raw env_file values (including $ and quotes).
"${compose[@]}" config --quiet
for registry in "${BACKEND_IMAGE%%/*}" "${EDGE_IMAGE%%/*}"; do
  aws ecr get-login-password | docker login --username AWS --password-stdin "$registry" >/dev/null
done
"${compose[@]}" pull
"${compose[@]}" run --rm --no-deps edge nginx -t
"${compose[@]}" up -d --wait --wait-timeout 180
# Also replace edge so its dependency and resolved backend address are current.
"${compose[@]}" up -d --no-deps --force-recreate --wait --wait-timeout 60 edge
curl --fail --silent --show-error --resolve bharatchat.in:443:127.0.0.1 https://bharatchat.in/ -o /dev/null
echo 'Deployment containers are healthy. Complete the release gates before admitting users.'
