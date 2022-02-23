$ErrorActionPreference = "Stop"

Write-Output "Installing golang..."
choco install -y golang
If ($lastexitcode -ne 0) { Exit $lastexitcode }

# Make `Update-SessionEnvironment` available
Write-Output "Importing the choco profile module..."
$ChocolateyInstall = Convert-Path "$((Get-Command choco).path)\..\.."
Import-Module "$ChocolateyInstall\helpers\chocolateyProfile.psm1"

Write-Output "Refreshing the current session environment..."
Update-SessionEnvironment

go version
If ($lastexitcode -ne 0) { Exit $lastexitcode }