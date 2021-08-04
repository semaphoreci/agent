#!/bin/bash

SEMAPHORE_DIRECTORY=/opt/semaphore

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

# TODO: allow version to be specified
agent_version=latest

# Other possible values are: Linux_i386 and Darwin_x86_64
read -p "Enter os [default: Linux_x86_64]: " agent_os
agent_os="${agent_os:=Linux_x86_64}"

echo "Creating $SEMAPHORE_DIRECTORY directory..."
sudo mkdir -p $SEMAPHORE_DIRECTORY
sudo chown admin:admin $SEMAPHORE_DIRECTORY

agent_url="https://github.com/semaphoreci/agent/releases/latest/download/agent_$agent_os.tar.gz"
echo "Downloading $agent_url..."
status_code=$(curl -w "%{http_code}" -sL $agent_url -o $SEMAPHORE_DIRECTORY/agent.tar.gz)
if [[ $status_code -ne "200" ]]; then
  echo "Error downloading agent: $status_code"
  exit 1
fi

echo "Extracting $SEMAPHORE_DIRECTORY/agent.tar.gz..."
tar -xf $SEMAPHORE_DIRECTORY/agent.tar.gz -C $SEMAPHORE_DIRECTORY

SYSTEMD_SERVICE=$(cat <<-END
[Unit]
Description=Semaphore agent
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=5
User=admin
WorkingDirectory=/opt/semaphore
ExecStart=/opt/semaphore/agent start --endpoint $organization.semaphoreci.com --token $registration_token

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