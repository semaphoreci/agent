$ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = "Stop"

#
# Download and install toolbox
#
$ToolboxDirectory="$HOME\.toolbox"

Write-Output "Toolbox will be installed at $ToolboxDirectory."
if (Test-Path $ToolboxDirectory) {
  Write-Output "Toolbox was already installed at $ToolboxDirectory. Overriding it..."
  Remove-Item -Path $ToolboxDirectory -Force -Recurse
}

Write-Output "Downloading toolbox..."
Invoke-WebRequest "https://github.com/semaphoreci/toolbox/releases/latest/download/self-hosted-windows.tar" -OutFile toolbox.tar
if (-not (Test-Path "toolbox.tar")) {
  Write-Output "Error downloading toolbox"
  Exit 1
}

Write-Output "Unpacking toolbox..."
tar -xf toolbox.tar -C $HOME
if (-not (Test-Path "$HOME\toolbox")) {
  Write-Output "Error unpacking toolbox"
  Exit 1
}

Rename-Item "$HOME\toolbox" $ToolboxDirectory
if (-not (Test-Path "$HOME\.toolbox")) {
  Write-Output "Error renaming toolbox directory"
  Exit 1
}

Write-Output "Installing toolbox..."
& "$ToolboxDirectory\install-toolbox.ps1"
if (-not $?) {
  Write-Output "Error installing toolbox"
  Exit 1
}

Remove-Item toolbox.tar

# TODO: create agent config
# TODO: create nssm service for agent
