#!/usr/bin/env bash
# Installed by bootstrap-host.sh; Docker must use its iptables firewall backend.
set -euo pipefail

valid_tester() {
  local address=$1 part
  [[ $address =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/32$ ]] || return 1
  local -a octets
  IFS=. read -r -a octets <<< "${address%/32}"
  for part in "${octets[@]}"; do
    [[ $part == 0 || $part != 0* ]] && ((10#$part <= 255)) || return 1
  done
  ((10#${octets[0]} > 0 && 10#${octets[0]} < 224 && 10#${octets[0]} != 127))
}

render_rules() {
  local tester=$1 interface=$2
  valid_tester "$tester" || return 2
  [[ $interface =~ ^[a-zA-Z0-9_.:-]+$ ]] || return 2
  cat <<EOF
*filter
:BHARATCHAT-PILOT - [0:0]
-F BHARATCHAT-PILOT
-A BHARATCHAT-PILOT -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
-A BHARATCHAT-PILOT -i $interface -s $tester -p tcp -m multiport --dports 80,443 -j RETURN
-A BHARATCHAT-PILOT -i $interface -j DROP
-A BHARATCHAT-PILOT -j RETURN
COMMIT
EOF
}

# Also used by local tests: this mode renders rules without touching the firewall.
if [[ ${1:-} == --render && $# == 3 ]]; then
  render_rules "$2" "$3"
  exit
fi
[[ $# == 0 && $EUID == 0 ]] || { echo 'Run as root without arguments' >&2; exit 2; }
tester=$(< /etc/bharatchat/tester-cidr)
valid_tester "$tester" || { echo 'Invalid tester IPv4 /32' >&2; exit 2; }
interface=$(ip -json route get 1.1.1.1 | jq -er '.[0].dev')
iptables -w -nL DOCKER-USER >/dev/null
# --noflush preserves unrelated tables/chains. Only our owned chain is replaced.
render_rules "$tester" "$interface" | iptables-restore --wait --noflush
if ! iptables -w -C DOCKER-USER -j BHARATCHAT-PILOT 2>/dev/null; then
  iptables -w -I DOCKER-USER 1 -j BHARATCHAT-PILOT
fi
