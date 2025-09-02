#!/bin/bash

set -e
set -o pipefail

#
# Creates a 'semaphore-agent' systemd service.
# If it already exists, it will be overriden.
# SEMAPHORE_AGENT_START controls whether the service will be started as well.
#
create_systemd_service() {
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
Environment=SEMAPHORE_AGENT_LOG_FILE_PATH=$AGENT_INSTALLATION_DIRECTORY/agent.log

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
}

#
# Creates a 'com.semaphoreci.agent' launchd daemon, at /Library/LaunchDaemons.
# If it already exists, it will be overriden. SEMAPHORE_AGENT_START controls
# whether the daemon will be started as well.
#
create_launchd_daemon() {
  LAUNCHD_DAEMON_LABEL=com.semaphoreci.agent
  LAUNCHD_DAEMON=$(cat <<-END
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LAUNCHD_DAEMON_LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$AGENT_INSTALLATION_DIRECTORY/agent</string>
    <string>start</string>
    <string>--config-file</string>
    <string>$AGENT_CONFIG_PATH</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>SEMAPHORE_AGENT_LOG_FILE_PATH</key>
    <string>$AGENT_INSTALLATION_DIRECTORY/agent.log</string>
    <key>PATH</key>
    <string>/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin</string>
  </dict>
  <key>RunAtLoad</key>
  <false/>
  <key>KeepAlive</key>
  <dict>
    <key>Crashed</key>
    <true/>
  </dict>
  <key>UserName</key>
  <string>$SEMAPHORE_AGENT_INSTALLATION_USER</string>
  <key>WorkingDirectory</key>
  <string>$AGENT_INSTALLATION_DIRECTORY</string>
</dict>
</plist>
END
  )

  LAUNCHD_PATH=/Library/LaunchDaemons
  LAUNCHD_DAEMON_PATH=$LAUNCHD_PATH/$LAUNCHD_DAEMON_LABEL.plist

  echo "Creating $LAUNCHD_DAEMON_PATH..."

  if [[ -f "$LAUNCHD_DAEMON_PATH" ]]; then
    echo "launchd daemon already exists at $LAUNCHD_DAEMON_PATH. Overriding it..."
    echo "$LAUNCHD_DAEMON" > $LAUNCHD_DAEMON_PATH

    if [[ "$SEMAPHORE_AGENT_START" == "false" ]]; then
      echo "Not starting/restarting $LAUNCHD_DAEMON_LABEL."
    else
      echo "Bootout $LAUNCHD_DAEMON_LABEL..."
      launchctl bootout system $LAUNCHD_DAEMON_PATH || true

      echo "Bootstrap $LAUNCHD_DAEMON_LABEL..."
      launchctl bootstrap system $LAUNCHD_DAEMON_PATH

      echo "Kickstart $LAUNCHD_DAEMON_LABEL..."
      launchctl kickstart -k system/com.semaphoreci.agent
    fi
  else
    echo "$LAUNCHD_DAEMON" > $LAUNCHD_DAEMON_PATH

    if [[ "$SEMAPHORE_AGENT_START" == "false" ]]; then
      echo "Not starting $LAUNCHD_DAEMON_LABEL."
    else
      echo "Bootstrap $LAUNCHD_DAEMON_LABEL service..."
      launchctl bootstrap system $LAUNCHD_DAEMON_PATH

      echo "Kickstart $LAUNCHD_DAEMON_LABEL service..."
      launchctl kickstart -k system/com.semaphoreci.agent
    fi
  fi
}

# Find the toolbox URL based on operating system (linux/darwin) and architecture.
# It also considers SEMAPHORE_TOOLBOX_VERSION. If not set, it uses the latest version.
find_toolbox_url() {
  local os=$(echo $DIST | tr '[:upper:]' '[:lower:]')
  local tarball_name="self-hosted-${os}.tar"
  if [[ ${ARCH} =~ "arm" || ${ARCH} == "aarch64" ]]; then
    tarball_name="self-hosted-${os}-arm.tar"
  fi

  if [[ -z "${SEMAPHORE_TOOLBOX_VERSION}" ]]; then
    echo "https://github.com/semaphoreci/toolbox/releases/latest/download/${tarball_name}"
  else
    echo "https://github.com/semaphoreci/toolbox/releases/download/${SEMAPHORE_TOOLBOX_VERSION}/${tarball_name}"
  fi
}

#
# Main script
#

DIST=$(uname)
ARCH=$(uname -m)
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

toolbox_url=$(find_toolbox_url)
echo "Downloading toolbox from ${toolbox_url}..."
curl -sL ${toolbox_url} -o toolbox.tar

tar -xf toolbox.tar
mv toolbox $TOOLBOX_DIRECTORY

case $DIST in
  Darwin)
    sudo chown -R $SEMAPHORE_AGENT_INSTALLATION_USER $TOOLBOX_DIRECTORY
    ;;
  Linux)
    sudo chown -R $SEMAPHORE_AGENT_INSTALLATION_USER:$SEMAPHORE_AGENT_INSTALLATION_USER $TOOLBOX_DIRECTORY
    ;;
esac

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
case $DIST in
  Darwin)
    sudo chown $SEMAPHORE_AGENT_INSTALLATION_USER $AGENT_CONFIG_PATH
    ;;
  Linux)
    sudo chown $SEMAPHORE_AGENT_INSTALLATION_USER:$SEMAPHORE_AGENT_INSTALLATION_USER $AGENT_CONFIG_PATH
    ;;
esac

SEMAPHORE_AGENT_SYSTEMD_RESTART=${SEMAPHORE_AGENT_SYSTEMD_RESTART:-always}
SEMAPHORE_AGENT_SYSTEMD_RESTART_SEC=${SEMAPHORE_AGENT_SYSTEMD_RESTART_SEC:-60}

#
# Check if we can use some kind of service manager to run the agent.
# We use systemd for Linux, and launchd for MacOS.
#
case $DIST in
  Darwin)
    create_launchd_daemon
    ;;
  Linux)
    create_systemd_service
    ;;
  *)
    echo "$DIST is not supported. You can still start the agent with '$AGENT_INSTALLATION_DIRECTORY/agent start --config $AGENT_CONFIG_PATH'."
    ;;
esac

echo "Done."
