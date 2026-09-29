# Installs ModemPhone for the current user.
#
# Copies the built program to %ProgramFiles%\ModemPhone, puts it on the PATH for
# this user, and optionally registers it to start with Windows. No administrator
# rights are needed: the per-user Run key is used, not a service.

param(
  [switch]$NoStartAtLogin,
  [switch]$Quiet
)

$ErrorActionPreference = "Stop"

function Say($msg) { if (-not $Quiet) { Write-Host $msg } }

$root = $PSScriptRoot
$build = Join-Path $root "build.ps1"
$source = Join-Path $root "bin\modemphone.exe"

if (-not (Test-Path $build)) { throw "build.ps1 not found next to install.ps1" }

Say "Building ModemPhone..."
& $build -Test
if ($LASTEXITCODE -ne 0) { throw "the build failed" }
if (-not (Test-Path $source)) { throw "the build produced no executable" }

# Prefer the machine-wide location, but fall back to the per-user one when
# Program Files is not writable, so the install works without elevation. The
# per-user path is the standard one for an app installed without admin rights.
$machineDir = Join-Path $env:ProgramFiles "ModemPhone"
$userDir = Join-Path $env:LOCALAPPDATA "Programs\ModemPhone"

function Test-Writable($dir) {
  try {
    New-Item -ItemType Directory -Force -Path $dir -ErrorAction Stop | Out-Null
    $probe = Join-Path $dir ".write-probe"
    Set-Content -Path $probe -Value "x" -ErrorAction Stop
    Remove-Item $probe -Force -ErrorAction SilentlyContinue
    return $true
  } catch {
    return $false
  }
}

if (Test-Writable $machineDir) {
  $installDir = $machineDir
} else {
  $installDir = $userDir
  Say "Program Files is not writable, installing per-user instead."
}
$target = Join-Path $installDir "modemphone.exe"

# An install is in use if the app is running from it; stop it first so the
# copy is not refused.
$running = Get-Process modemphone -ErrorAction SilentlyContinue |
  Where-Object { $_.Path -and $_.Path.StartsWith($installDir, [System.StringComparison]::OrdinalIgnoreCase) }
if ($running) {
  Say "Closing ModemPhone..."
  $running | Stop-Process -Force
  Start-Sleep -Milliseconds 800
}

Say "Installing to $installDir ..."
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item $source $target -Force

# Per-user PATH, so the program can be run from a terminal by name.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = @()
if ($userPath) { $parts = $userPath -split ";" | Where-Object { $_ } }
if ($parts -notcontains $installDir) {
  $newPath = (@($parts) + $installDir) -join ";"
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
  Say "Added to your PATH."
  # Make it usable in this session too.
  if (($env:Path -split ";") -notcontains $installDir) { $env:Path = "$env:Path;$installDir" }
}

# The Start-menu shortcut, so it is findable without the terminal.
$programs = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
New-Item -ItemType Directory -Force -Path $programs | Out-Null
$shortcut = Join-Path $programs "ModemPhone.lnk"
$shell = New-Object -ComObject WScript.Shell
$lnk = $shell.CreateShortcut($shortcut)
$lnk.TargetPath = $target
$lnk.WorkingDirectory = $installDir
$lnk.Description = "Read, send and manage SMS on a 4G modem"
$lnk.Save()
Say "Added a Start-menu shortcut."

# Startup registration, through the app's own code path so it matches exactly
# what the Settings toggle does.
if ($NoStartAtLogin) {
  Say "Skipping startup registration (use -NoStartAtLogin to leave it off)."
} else {
  $key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
  New-Item -Path $key -Force | Out-Null
  Set-ItemProperty -Path $key -Name "ModemPhone" -Value ('"' + $target + '"') -Type String
  Say "Registered to start with Windows (you can turn this off in the app's Settings)."
}

Say ""
Say "Installed: $target"
Say "Run it with:  modemphone"
