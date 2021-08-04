#!/bin/bash

while getopts "o:t:" opt; do
  case $opt in
    o)
      organization=$OPTARG
      ;;
    t)
      agent_registration_token=$OPTARG
      ;;
    \?)
      echo "Invalid option: -$OPTARG"
      exit 1
      ;;
    :)
      echo "Option -$OPTARG requires an argument."
      exit 1
      ;;
  esac
done

if [[ -z $organization ]]; then
  echo "Organization not set. Please provide the organization name using the -o option"
  exit 1
fi

if [[ -z $agent_registration_token ]]; then
  echo "Agent registration token not set. Please provide the agent registration token using the -t option"
  exit 1
fi

echo "Creating /opt/semaphore/agent directory..."
sudo mkdir -p /opt/semaphore/agent
sudo chown admin:admin /opt/semaphore/agent/
cd /opt/semaphore/agent

echo "Downloading agent..."
curl -L https://github.com/semaphoreci/agent/releases/download/v2.0.5-alpha/agent -o agent
chmod +x agent

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
WorkingDirectory=/opt/semaphore/agent
ExecStart=/opt/semaphore/agent/agent start --endpoint $organization.semaphoreci.com --token $agent_registration_token

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