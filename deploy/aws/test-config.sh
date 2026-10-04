#!/usr/bin/env bash
# Run inside the pinned Nginx image with bash, jq, openssl, and shellcheck installed.
set -euo pipefail
umask 077
directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
temporary=$(mktemp -d)
cleanup() {
  rm -f -- "$temporary/valid.json" "$temporary/backend.env" "$temporary/nginx.conf" \
    "$temporary/fullchain.pem" "$temporary/privkey.pem" "$temporary/nginx.pid"
  rmdir -- "$temporary"
}
trap cleanup EXIT

for script in "$directory"/*.sh; do bash -n "$script"; done
shellcheck "$directory"/*.sh
# Rendering exercises validation without root access or any firewall mutation.
rules=$(bash "$directory/host-firewall.sh" --render 203.0.113.10/32 eth0)
grep -Fq -- '-A BHARATCHAT-PILOT -i eth0 -j DROP' <<< "$rules"
grep -Fq -- '-s 203.0.113.10/32 -p tcp -m multiport --dports 80,443' <<< "$rules"
for cidr in 0.0.0.0/0 999.1.1.1/32 01.2.3.4/32 127.0.0.1/32 '203.0.113.1/32;echo unsafe'; do
  if bash "$directory/host-firewall.sh" --render "$cidr" eth0 >/dev/null; then
    echo 'Invalid firewall source accepted' >&2; exit 1
  fi
done
if bash "$directory/host-firewall.sh" --render 203.0.113.1/32 'eth0 -j ACCEPT' >/dev/null; then
  echo 'Invalid network interface accepted' >&2; exit 1
fi
jq -n '{
  JWT_ACCESS_SECRET: ("x" * 48),
  POSTGRES_HOST: "example.ap-south-1.rds.amazonaws.com", POSTGRES_USER: "bharatchat",
  POSTGRES_PASSWORD: "test-$literal=\"quote\"", POSTGRES_DB: "bharatchat",
  REDIS_HOST: "example.aps1.cache.amazonaws.com", REDIS_PASSWORD: "test-cache-secret",
  REDIS_TLS_SERVER_NAME: "example.aps1.cache.amazonaws.com",
  OTP_MSG91_AUTH_KEY: "not-a-real-auth-key", OTP_MSG91_TEMPLATE_ID: "0123456789abcdef",
  OTP_ALLOWED_PHONES: "+12025550100,+12025550101"
}' > "$temporary/valid.json"
jq -er -f "$directory/secret-env.jq" "$temporary/valid.json" > "$temporary/backend.env"
# Credentials must retain literal dollar signs and quotes.
# shellcheck disable=SC2016
grep -Fqx 'POSTGRES_PASSWORD=test-$literal="quote"' "$temporary/backend.env"
for mutation in \
  'del(.JWT_ACCESS_SECRET)' \
  '.JWT_ACCESS_SECRET = "weak"' \
  '.REQUIRE_E2EE = "false"' \
  '.MESSAGING_ENABLED = "true"' \
  'del(.OTP_ALLOWED_PHONES)' \
  '.OTP_ALLOWED_PHONES = ""' \
  '.OTP_ALLOWED_PHONES = "+12025550100, +12025550101"' \
  '.OTP_ALLOWED_PHONES = "+12025550100,+12025550100"' \
  '.OTP_ALLOWED_PHONES = ([range(21) | "+12025550" + (100 + . | tostring)] | join(","))' \
  '.POSTGRES_PASSWORD = "secret\nAPP_ENV=development"' \
  '.REDIS_TLS_SERVER_NAME = "other.cache.amazonaws.com"' \
  '.POSTGRES_HOST = "localhost"' \
  '.OTP_MSG91_AUTH_KEY = null'; do
  if jq "$mutation" "$temporary/valid.json" | jq -er -f "$directory/secret-env.jq" >/dev/null 2>&1; then
    printf 'Secret validation unexpectedly accepted: %s\n' "$mutation" >&2
    exit 1
  fi
done
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=bharatchat.in \
  -addext 'subjectAltName=DNS:bharatchat.in,DNS:api.bharatchat.in' \
  -keyout "$temporary/privkey.pem" -out "$temporary/fullchain.pem" >/dev/null 2>&1
sed "s|/etc/letsencrypt/live/bharatchat.in|$temporary|g; s|/var/run/nginx.pid|$temporary/nginx.pid|g" \
  "$directory/nginx.conf" > "$temporary/nginx.conf"
nginx -t -c "$temporary/nginx.conf"
printf 'PASS: shell checks, secret validation, literal credentials, Nginx TLS configuration\n'
