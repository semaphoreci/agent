#!/bin/bash

set -e
set -o pipefail

if [[ "$EUID" -ne 0 ]]; then
  echo "Please run with sudo."
  exit 1
fi

read -p "Enter organization: " organization
if [[ -z $organization ]]; then
  echo "Organization cannot be empty."
  exit 1
fi

read -p "Enter registration token: " registration_token
if [[ -z $registration_token ]]; then
  echo "Registration token cannot be empty."
  exit 1
fi

install_directory=$(pwd)
logged_in_user=$(logname)
read -p "Enter user [$logged_in_user]: " install_user
install_user="${install_user:=$logged_in_user}"

if ! id "$install_user" &>/dev/null; then
  echo "User $install_user does not exist. Exiting..."
  exit 1
fi

AGENT_CONFIG=$(cat <<-END
endpoint: "$organization.semaphoreci.com"
token: "$registration_token"
no-https: false
shutdown-hook-path: ""
disconnect-after-job: false
env-vars: []
files: []
fail-on-missing-files: false
END
)

AGENT_CONFIG_PATH="$install_directory/config.yaml"
echo "Creating agent config file at $AGENT_CONFIG_PATH..."
echo "$AGENT_CONFIG" > $AGENT_CONFIG_PATH

SYSTEMD_SERVICE=$(cat <<-END
[Unit]
Description=Semaphore agent
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=5
User=$install_user
WorkingDirectory=$install_directory
ExecStart=$install_directory/agent start --config-file $AGENT_CONFIG_PATH

[Install]
WantedBy=multi-user.target
END
)

SYSTEMD_PATH=/etc/systemd/system
SERVICE_NAME=semaphore-agent
SYSTEMD_SERVICE_PATH=$SYSTEMD_PATH/$SERVICE_NAME.service

echo "Creating $SYSTEMD_SERVICE_PATH..."
echo "$SYSTEMD_SERVICE" > $SYSTEMD_SERVICE_PATH

echo "Starting semaphore-agent service..."
systemctl start semaphore-agent

echo "Done."