$ErrorActionPreference = "Stop"

function Add-To-Path {
  param (
    $Path
  )

  $Content = "if (-not (`$env:Path.Split(';').Contains('$Path'))) { `$env:Path = '$Path;' + `$env:Path }"
  $ProfilePath = $profile.AllUsersAllHosts
  Write-Output "Adding $Path to `$env:Path, using profile $ProfilePath..."

  if (-not (Test-Path $profilePath)) {
    Write-Output "$ProfilePath does not exist. Creating it..."
    New-Item $ProfilePath > $null
    Set-Content $ProfilePath $Content
  } else {
    Add-Content -Path $ProfilePath -Value $Content
  }
}

Write-Output "Installing golang..."
choco install -y golang
If ($lastexitcode -ne 0) { Exit $lastexitcode }

Write-Output "Installing Git for Windows"
choco install -y git --version 2.31.0
If ($lastexitcode -ne 0) { Exit $lastexitcode }

Add-To-Path -Path "C:\Program Files\Go\bin"
Add-To-Path -Path "C:\Program Files\Git\bin"

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
