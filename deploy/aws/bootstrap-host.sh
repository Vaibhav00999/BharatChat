#!/usr/bin/env bash
# Fresh EC2 Ubuntu 24.04 only. Run through SSM, never over an SSH connection.
set -euo pipefail
umask 077
[[ $EUID == 0 && $# == 1 && -z ${SSH_CONNECTION:-} ]] || {
  echo 'Usage through SSM: sudo bash bootstrap-host.sh TESTER_IPV4/32' >&2; exit 2;
}
directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
tester=$1
bash "$directory/host-firewall.sh" --render "$tester" eth0 >/dev/null
# shellcheck disable=SC1091
source /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 24.04 && $(dpkg --print-architecture) == amd64 ]] || {
  echo 'Expected Ubuntu 24.04 amd64' >&2; exit 2;
}
if command -v docker >/dev/null || [[ -e /etc/bharatchat/tester-cidr || -e /etc/docker/daemon.json ]]; then
  echo 'Fresh host required; existing Docker/firewall configuration will not be overwritten' >&2
  exit 2
fi
if command -v ufw >/dev/null && ufw status | grep -Fq 'Status: active'; then
  echo 'Existing active firewall requires an operator review; refusing to change it' >&2
  exit 2
fi
systemctl is-active --quiet snap.amazon-ssm-agent.amazon-ssm-agent.service || {
  echo 'Confirm a working Session Manager connection before installing the firewall' >&2; exit 2;
}
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl jq openssl ufw iptables unzip snapd unattended-upgrades
install -d -m 0755 /etc/apt/keyrings
curl --fail --silent --show-error --proto '=https' --tlsv1.2 \
  https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod 0644 /etc/apt/keyrings/docker.asc
cat > /etc/apt/sources.list.d/docker.sources <<'EOF'
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: noble
Components: stable
Architectures: amd64
Signed-By: /etc/apt/keyrings/docker.asc
EOF
chmod 0644 /etc/apt/sources.list.d/docker.sources
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
# AWS publishes this signed snap. Credentials come from the EC2 role, not aws configure.
snap install aws-cli --classic
install -d -m 0700 /etc/bharatchat
install -d -m 0755 /opt/bharatchat /etc/systemd/system/docker.service.d
printf '%s\n' "$tester" > /etc/bharatchat/tester-cidr
install -m 0755 "$directory/host-firewall.sh" /usr/local/sbin/bharatchat-firewall
cat > /etc/systemd/system/docker.service.d/bharatchat-firewall.conf <<'EOF'
[Service]
ExecStartPost=/usr/local/sbin/bharatchat-firewall
EOF
ufw default deny incoming
ufw default allow outgoing
ufw allow from "$tester" to any port 80 proto tcp
ufw allow from "$tester" to any port 443 proto tcp
ufw --force enable
systemctl daemon-reload
systemctl enable docker
systemctl restart docker
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF
systemctl enable --now unattended-upgrades
# Do not install host Nginx: the reviewed edge container owns 80/443.
docker compose version
/snap/bin/aws --version
ufw status
iptables -w -nL BHARATCHAT-PILOT
echo 'Host tools and prelaunch firewall installed. Certificates, datastores, and deployment remain separate steps.'
