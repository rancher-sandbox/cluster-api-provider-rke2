#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

setup_infrastructure () {
  if [[ "$1" == "aws" ]]; then
    zypper --gpg-auto-import-keys --non-interactive install awscli amazon-ssm-agent
    systemctl enable amazon-ssm-agent
  fi
}

configure_systemd () {
  echo "Enabling required systemd services"

  systemctl enable sshd
  systemctl enable cloud-final
  systemctl enable cloud-config
  systemctl enable cloud-init
  systemctl enable cloud-init-local
  
  systemctl stop cloud-final
  systemctl stop cloud-config
  systemctl stop cloud-init
  systemctl stop cloud-init-local
}

configure_cloudinit () {
  echo "Configuring cloud-init"

  cloud-init clean -s -l
  rm -f /var/log/cloud-init*
  rm -rf /var/lib/cloud/*

  mkdir -p /usr/lib/python3/dist-packages/cloudinit
  mv /tmp/features.py /usr/lib/python3.6/site-packages/cloudinit/features.py
}

cleanup_ssh_keys () {
  echo "Cleaning up SSH keys"

  rm -rf /etc/ssh/ssh_host_*
  rm -rf /root/.ssh/authorized_keys
  rm -rf /home/ec2-user/.ssh/authorized_keys
}

echo "Provisioning instance for $2"

echo "Install required packages"

zypper --gpg-auto-import-keys ref && \
zypper --gpg-auto-import-keys --non-interactive install \
        curl \
        openssh-server \
        cloud-init \
        systemd \
        openssh \

echo "Install RKE2 components"

mkdir -p /opt/rke2-artifacts
curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst https://github.com/rancher/rke2/releases/download/v${1}/rke2-images.linux-amd64.tar.zst
curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz https://github.com/rancher/rke2/releases/download/v${1}/rke2.linux-amd64.tar.gz
curl -sfL -o /opt/rke2-artifacts/sha256sum-amd64.txt https://github.com/rancher/rke2/releases/download/v${1}/sha256sum-amd64.txt
curl -sfL -o /opt/install.sh https://get.rke2.io

configure_systemd 
configure_cloudinit
cleanup_ssh_keys
setup_infrastructure $2

echo "Done"