$ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = "Stop"
$InstallationDirectory = $PSScriptRoot

#
# Assert required variables are set
#
if (Test-Path env:SemaphoreEndpoint) {
  $SemaphoreEndpoint = $env:SemaphoreEndpoint
} else {
  if (-not (Test-Path env:SemaphoreOrganization)) {
    Write-Warning 'Either $env:SemaphoreOrganization or $env:SemaphoreEndpoint needs to be specified. Exiting...'
    Exit 1
  }

  $SemaphoreEndpoint = "$env:SemaphoreOrganization.semaphoreci.com"
  Write-Warning "`$env:SemaphoreEndpoint not set, using '$SemaphoreEndpoint'"
}

if (-not (Test-Path env:SemaphoreRegistrationToken)) {
  Write-Warning 'Registration token cannot be empty, set $env:SemaphoreRegistrationToken. Exiting...'
  Exit 1
}

#
# Set defaults in case some variables are not set
#
if (Test-Path env:SemaphoreAgentDisconnectAfterJob) {
  $DisconnectAfterJob = $env:SemaphoreAgentDisconnectAfterJob
} else {
  $DisconnectAfterJob = "false"
}

if (Test-Path env:SemaphoreAgentDisconnectAfterIdleTimeout) {
  $DisconnectAfterIdleTimeout = $env:SemaphoreAgentDisconnectAfterIdleTimeout
} else {
  $DisconnectAfterIdleTimeout = 0
}

#
# Download and unpack toolbox
#
$ToolboxDirectory = Join-Path $HOME ".toolbox"
$InstallScriptPath = Join-Path $ToolboxDirectory "install-toolbox.ps1"

Write-Output "> Toolbox will be installed at $ToolboxDirectory."
if (Test-Path $ToolboxDirectory) {
  Write-Output "> Toolbox already installed at $ToolboxDirectory. Overriding it..."
  Remove-Item -Path $ToolboxDirectory -Force -Recurse
}

if (Test-Path env:SemaphoreToolboxVersion) {
  Write-Output "> Downloading and unpacking env:SemaphoreToolboxVersion toolbox..."
  Invoke-WebRequest "https://github.com/semaphoreci/toolbox/releases/download/$env:SemaphoreToolboxVersion/self-hosted-windows.tar" -OutFile toolbox.tar
} else {
  Write-Output '> $env:SemaphoreToolboxVersion is not set. Downloading and unpacking latest toolbox...'
  Invoke-WebRequest "https://github.com/semaphoreci/toolbox/releases/latest/download/self-hosted-windows.tar" -OutFile toolbox.tar
}

tar.exe -xf toolbox.tar -C $HOME
Rename-Item "$HOME\toolbox" $ToolboxDirectory
Remove-Item toolbox.tar -Force

#
# Install toolbox
#
Write-Output "> Installing toolbox..."
& $InstallScriptPath

#
# Create agent config in current directory
#
$AgentConfig = @"
endpoint: "$SemaphoreEndpoint"
token: "$env:SemaphoreRegistrationToken"
no-https: false
shutdown-hook-path: "$env:SemaphoreAgentShutdownHook"
disconnect-after-job: $DisconnectAfterJob
disconnect-after-idle-timeout: $DisconnectAfterIdleTimeout
env-vars: []
files: []
fail-on-missing-files: false
"@

$AgentConfigPath = Join-Path $InstallationDirectory "config.yaml"
if (Test-Path $AgentConfigPath) {
  Write-Output "> Agent configuration file already exists in $AgentConfigPath. Overriding it..."
  Remove-Item -Path $AgentConfigPath -Force -Recurse
}

New-Item -ItemType File -Path $AgentConfigPath > $null
Set-Content -Path $AgentConfigPath -Value $AgentConfig

Write-Output "> Successfully installed the agent in $InstallationDirectory."
Write-Output "
  Start the agent with: $InstallationDirectory\agent.exe start --config-file $AgentConfigPath
"
