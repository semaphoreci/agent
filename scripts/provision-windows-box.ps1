$ErrorActionPreference = "Stop"

Write-Output "Installing golang..."
choco install -y golang
If ($lastexitcode -ne 0) { Exit $lastexitcode }

Write-Output "Installing Git for Windows"
choco install -y git --version 2.31.0
If ($lastexitcode -ne 0) { Exit $lastexitcode }

Write-Output "Importing the choco profile module..."
$ChocolateyInstall = Convert-Path "$((Get-Command choco).path)\..\.."
Import-Module "$ChocolateyInstall\helpers\chocolateyProfile.psm1"
Write-Output "Refreshing the current session environment..."
Update-SessionEnvironment

go version
If ($lastexitcode -ne 0) { Exit $lastexitcode }

# no mismatched line endings
git config --system core.autocrlf false
If ($lastexitcode -ne 0) { Exit $lastexitcode }
