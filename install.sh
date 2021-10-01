#!/bin/bash

set -e
set -o pipefail

AGENT_INSTALLATION_DIRECTORY=$(pwd)
LOGGED_IN_USER=$(logname)
TOOLBOX_VERSION="v1.14.6"

if [[ "$EUID" -ne 0 ]]; then
  echo "Please run with sudo."
  exit 1
fi

if [[ -z $SEMAPHORE_ORGANIZATION ]]; then
  read -p "Enter organization: " SEMAPHORE_ORGANIZATION
  if [[ -z $SEMAPHORE_ORGANIZATION ]]; then
    echo "Organization cannot be empty."
    exit 1
  fi
fi

if [[ -z $SEMAPHORE_REGISTRATION_TOKEN ]]; then
  read -p "Enter registration token: " SEMAPHORE_REGISTRATION_TOKEN
  if [[ -z $SEMAPHORE_REGISTRATION_TOKEN ]]; then
    echo "Registration token cannot be empty."
    exit 1
  fi
fi

read -p "Enter user [$LOGGED_IN_USER]: " AGENT_INSTALLATION_USER
AGENT_INSTALLATION_USER="${AGENT_INSTALLATION_USER:=$LOGGED_IN_USER}"

if ! id "$AGENT_INSTALLATION_USER" &>/dev/null; then
  echo "User $AGENT_INSTALLATION_USER does not exist. Exiting..."
  exit 1
fi

#
# Download toolbox
#
echo "Installing toolbox..."
USER_HOME_DIRECTORY=$(sudo -u $AGENT_INSTALLATION_USER -H bash -c 'echo $HOME')
curl -sL "https://github.com/semaphoreci/toolbox/releases/download/$TOOLBOX_VERSION/self-hosted-linux.tar" -o toolbox.tar
tar -xf toolbox.tar

mv toolbox $USER_HOME_DIRECTORY/.toolbox
sudo chown -R $AGENT_INSTALLATION_USER:$AGENT_INSTALLATION_USER $USER_HOME_DIRECTORY/.toolbox

sudo -u $AGENT_INSTALLATION_USER -H bash $USER_HOME_DIRECTORY/.toolbox/install-toolbox
echo "source ~/.toolbox/toolbox" >> $USER_HOME_DIRECTORY/.bash_profile
rm toolbox.tar

#
# Create agent config
#
AGENT_CONFIG=$(cat <<-END
endpoint: "$SEMAPHORE_ORGANIZATION.semaphoreci.com"
token: "$SEMAPHORE_REGISTRATION_TOKEN"
no-https: false
shutdown-hook-path: ""
disconnect-after-job: false
env-vars: []
files: []
fail-on-missing-files: false
END
)

AGENT_CONFIG_PATH="$AGENT_INSTALLATION_DIRECTORY/config.yaml"
echo "Creating agent config file at $AGENT_CONFIG_PATH..."
echo "$AGENT_CONFIG" > $AGENT_CONFIG_PATH
sudo chown $AGENT_INSTALLATION_USER:$AGENT_INSTALLATION_USER $AGENT_CONFIG_PATH

#
# Create systemd service
#
SYSTEMD_SERVICE=$(cat <<-END
[Unit]
Description=Semaphore agent
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=5
User=$AGENT_INSTALLATION_USER
WorkingDirectory=$AGENT_INSTALLATION_DIRECTORY
ExecStart=$AGENT_INSTALLATION_DIRECTORY/agent start --config-file $AGENT_CONFIG_PATH

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