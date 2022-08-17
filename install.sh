#!/bin/bash

set -e
set -o pipefail

AGENT_INSTALLATION_DIRECTORY="$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

if [[ "$EUID" -ne 0 ]]; then
  echo "Please run with sudo."
  exit 1
fi

if [[ -z $SEMAPHORE_ENDPOINT ]]; then
  if [[ -z $SEMAPHORE_ORGANIZATION ]]; then
    read -p "Enter organization: " SEMAPHORE_ORGANIZATION
    if [[ -z $SEMAPHORE_ORGANIZATION ]]; then
      echo "Organization cannot be empty."
      exit 1
    fi
  fi

  SEMAPHORE_ENDPOINT="$SEMAPHORE_ORGANIZATION.semaphoreci.com"
fi

if [[ -z $SEMAPHORE_REGISTRATION_TOKEN ]]; then
  read -p "Enter registration token: " SEMAPHORE_REGISTRATION_TOKEN
  if [[ -z $SEMAPHORE_REGISTRATION_TOKEN ]]; then
    echo "Registration token cannot be empty."
    exit 1
  fi
fi

if [[ -z $SEMAPHORE_AGENT_INSTALLATION_USER ]]; then
  LOGGED_IN_USER=$(logname)
  read -p "Enter user [$LOGGED_IN_USER]: " SEMAPHORE_AGENT_INSTALLATION_USER
  SEMAPHORE_AGENT_INSTALLATION_USER="${SEMAPHORE_AGENT_INSTALLATION_USER:=$LOGGED_IN_USER}"
fi

if ! id "$SEMAPHORE_AGENT_INSTALLATION_USER" &>/dev/null; then
  echo "User $SEMAPHORE_AGENT_INSTALLATION_USER does not exist. Exiting..."
  exit 1
fi

#
# Download toolbox
#
USER_HOME_DIRECTORY=$(sudo -u $SEMAPHORE_AGENT_INSTALLATION_USER -H bash -c 'echo $HOME')
TOOLBOX_DIRECTORY="$USER_HOME_DIRECTORY/.toolbox"
if [[ -d "$TOOLBOX_DIRECTORY" ]]; then
  echo "Toolbox was already installed at $TOOLBOX_DIRECTORY. Overriding it..."
  rm -rf "$TOOLBOX_DIRECTORY"
fi

if [[ -z "${SEMAPHORE_TOOLBOX_VERSION}" ]]; then
  echo "SEMAPHORE_TOOLBOX_VERSION is not set. Installing latest toolbox..."
  curl -sL "https://github.com/semaphoreci/toolbox/releases/latest/download/self-hosted-linux.tar" -o toolbox.tar
else
  echo "Installing ${SEMAPHORE_TOOLBOX_VERSION} toolbox..."
  curl -sL "https://github.com/semaphoreci/toolbox/releases/download/${SEMAPHORE_TOOLBOX_VERSION}/self-hosted-linux.tar" -o toolbox.tar
fi

tar -xf toolbox.tar
mv toolbox $TOOLBOX_DIRECTORY
sudo chown -R $SEMAPHORE_AGENT_INSTALLATION_USER:$SEMAPHORE_AGENT_INSTALLATION_USER $TOOLBOX_DIRECTORY

sudo -u $SEMAPHORE_AGENT_INSTALLATION_USER -H bash $TOOLBOX_DIRECTORY/install-toolbox
echo "source ~/.toolbox/toolbox" >> $USER_HOME_DIRECTORY/.bash_profile
rm toolbox.tar

#
# Create agent config
#
SEMAPHORE_AGENT_DISCONNECT_AFTER_JOB=${SEMAPHORE_AGENT_DISCONNECT_AFTER_JOB:-false}
SEMAPHORE_AGENT_DISCONNECT_AFTER_IDLE_TIMEOUT=${SEMAPHORE_AGENT_DISCONNECT_AFTER_IDLE_TIMEOUT:-0}
AGENT_CONFIG=$(cat <<-END
endpoint: "$SEMAPHORE_ENDPOINT"
token: "$SEMAPHORE_REGISTRATION_TOKEN"
no-https: false
shutdown-hook-path: "$SEMAPHORE_AGENT_SHUTDOWN_HOOK"
disconnect-after-job: $SEMAPHORE_AGENT_DISCONNECT_AFTER_JOB
disconnect-after-idle-timeout: $SEMAPHORE_AGENT_DISCONNECT_AFTER_IDLE_TIMEOUT
env-vars: []
files: []
fail-on-missing-files: false
END
)

AGENT_CONFIG_PATH="$AGENT_INSTALLATION_DIRECTORY/config.yaml"
echo "Creating agent config file at $AGENT_CONFIG_PATH..."
echo "$AGENT_CONFIG" > $AGENT_CONFIG_PATH
sudo chown $SEMAPHORE_AGENT_INSTALLATION_USER:$SEMAPHORE_AGENT_INSTALLATION_USER $AGENT_CONFIG_PATH

SEMAPHORE_AGENT_SYSTEMD_RESTART=${SEMAPHORE_AGENT_SYSTEMD_RESTART:-always}
SEMAPHORE_AGENT_SYSTEMD_RESTART_SEC=${SEMAPHORE_AGENT_SYSTEMD_RESTART_SEC:-60}

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
Restart=$SEMAPHORE_AGENT_SYSTEMD_RESTART
RestartSec=$SEMAPHORE_AGENT_SYSTEMD_RESTART_SEC
User=$SEMAPHORE_AGENT_INSTALLATION_USER
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

if [[ -f "$SYSTEMD_SERVICE_PATH" ]]; then
  echo "systemd service already exists at $SYSTEMD_SERVICE_PATH. Overriding it..."
  echo "$SYSTEMD_SERVICE" > $SYSTEMD_SERVICE_PATH
  systemctl daemon-reload
  if [[ "$SEMAPHORE_AGENT_START" == "false" ]]; then
    echo "Not restarting agent."
  else
    echo "Restarting semaphore-agent service..."
    systemctl restart semaphore-agent
  fi
else
  echo "$SYSTEMD_SERVICE" > $SYSTEMD_SERVICE_PATH
  if [[ "$SEMAPHORE_AGENT_START" == "false" ]]; then
    echo "Not starting agent."
  else
    echo "Starting semaphore-agent service..."
    systemctl start semaphore-agent
  fi
fi

echo "Done."