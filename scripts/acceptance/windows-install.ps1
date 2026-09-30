<#
.SYNOPSIS
The Windows installer's acceptance, run by hand on a Windows machine
(agentry-desktop#83; plan agentry-notes/plans/desktop-installers.md §8):
install release N-1, write known settings and a Credential Manager entry,
install release N over it, prove every byte and the credential survive,
uninstall silently and prove they still do; then, with -RemoveSettings, run
the removal the uninstaller's tick box runs and prove exactly those things
went. A silent uninstall also removes the Docker images Agentry put on the
machine, since "Keep my Docker images" is off by default (agentry-desktop#424):
run it where losing those images is fine; the next run fetches them again. Until #55 gives CI a Windows runner this is the operator's step, and
the issue comment says whether it ran.

.EXAMPLE
  .\windows-install.ps1 -Old .\agentry-desktop-0.1.1-windows-amd64-setup.exe -New .\agentry-desktop-0.2.0-windows-amd64-setup.exe
  .\windows-install.ps1 -Old $setup -New $setup -RemoveSettings   # the same file twice is a reinstall

Run in a user session, never elevated: a UAC prompt anywhere is a failure of
D11 and the script says so if it finds itself running as administrator.
Uses a throwaway credential (agentry:acceptance.example.invalid) and never
reads or writes a real one; the settings it writes are its own files, so
run it on a machine (or account) whose Agentry configuration you can lose,
because -RemoveSettings deletes %USERPROFILE%\.config\agentry,
%USERPROFILE%\.agentry\runner and every agentry:* credential.
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$Old,
  [Parameter(Mandatory)][string]$New,
  [switch]$RemoveSettings
)
$ErrorActionPreference = 'Stop'

function Fail([string]$m) { Write-Host "FAIL: $m" -ForegroundColor Red; exit 1 }
function Ok([string]$m) { Write-Host "ok: $m" }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin) { Fail "running elevated; the installer must be proven from a plain user session (D11)" }

$instDir = Join-Path $env:LOCALAPPDATA 'Programs\Agentry Desktop'
$exe = Join-Path $instDir 'agentry-desktop.exe'
$uninst = Join-Path $instDir 'uninstall.exe'
$confDir = Join-Path $env:USERPROFILE '.config\agentry'
$runnerHome = Join-Path $env:USERPROFILE '.agentry\runner'
$credTarget = 'agentry:acceptance.example.invalid'
$uninstKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\SecureAgentryAgentry Desktop'

function Install([string]$setup) {
  $p = Start-Process -FilePath $setup -ArgumentList '/S' -Wait -PassThru
  if ($p.ExitCode -ne 0) { Fail "$setup /S exited $($p.ExitCode)" }
  if (-not (Test-Path $exe)) { Fail "$exe is not there after installing $setup" }
}

function Hashes() {
  $h = @{}
  foreach ($d in @($confDir, $runnerHome)) {
    if (Test-Path $d) {
      Get-ChildItem $d -Recurse -File | ForEach-Object { $h[$_.FullName] = (Get-FileHash $_.FullName -Algorithm SHA256).Hash }
    }
  }
  return $h
}

function CredentialPresent() {
  $out = cmdkey /list:$credTarget 2>&1 | Out-String
  return ($out -match [regex]::Escape($credTarget))
}

# 1. Release N-1.
Install $Old
Ok "installed $Old under $instDir"

# 2. Known configuration: our own files beside whatever is there, and a
#    throwaway credential in the service the two clients share.
New-Item -ItemType Directory -Force -Path (Join-Path $confDir 'keys'), (Join-Path $runnerHome 'profiles') | Out-Null
$stamp = [guid]::NewGuid().ToString()
Set-Content -Path (Join-Path $confDir 'acceptance-desktop.yaml') -Value "# acceptance $stamp`nonboarded: true`n"
Set-Content -Path (Join-Path $confDir 'acceptance-installed.yaml') -Value "# acceptance $stamp`n"
Set-Content -Path (Join-Path $runnerHome 'acceptance-policy.yaml') -Value "# acceptance $stamp`n"
cmdkey /generic:$credTarget /user:acceptance.example.invalid /pass:$stamp | Out-Null
if (-not (CredentialPresent)) { Fail "could not store the throwaway credential $credTarget" }
$before = Hashes
Ok "wrote $($before.Count) configuration files and the credential $credTarget"

# 3. Release N over it.
Install $New
$after = Hashes
foreach ($k in $before.Keys) {
  if (-not $after.ContainsKey($k)) { Fail "$k is gone after installing $New" }
  if ($after[$k] -ne $before[$k]) { Fail "$k changed after installing $New" }
}
if (-not (CredentialPresent)) { Fail "the credential $credTarget is gone after installing $New" }
$ver = (Get-ItemProperty $uninstKey -ErrorAction SilentlyContinue).DisplayVersion
$loc = (Get-ItemProperty $uninstKey -ErrorAction SilentlyContinue).InstallLocation
if (-not $loc) { Fail "no InstallLocation under $uninstKey" }
Ok "installed $New over it: every configuration byte and the credential survive; Apps & features says $ver at $loc"

if ($RemoveSettings) {
  # 4a. What the tick box runs, while the binary is still there.
  $p = Start-Process -FilePath $exe -ArgumentList '--remove-my-settings' -Wait -PassThru
  if ($p.ExitCode -ne 0) { Fail "agentry-desktop.exe --remove-my-settings exited $($p.ExitCode)" }
  if (Test-Path $confDir) { Fail "$confDir still exists after --remove-my-settings" }
  if (Test-Path $runnerHome) { Fail "$runnerHome still exists after --remove-my-settings" }
  if (CredentialPresent) { Fail "the credential $credTarget survived --remove-my-settings" }
  if (-not (Test-Path (Join-Path $env:USERPROFILE '.config'))) { Fail "%USERPROFILE%\.config itself is gone; only the agentry directory may go" }
  Ok "--remove-my-settings removed exactly $confDir, $runnerHome and the agentry credentials"
}

# 4b. The default uninstall.
$p = Start-Process -FilePath $uninst -ArgumentList '/S' -Wait -PassThru
if ($p.ExitCode -ne 0) { Fail "uninstall.exe /S exited $($p.ExitCode)" }
# The uninstaller copies itself to %TEMP% and finishes after the process
# returns; its Docker images step (agentry-desktop#424) waits on Docker for
# up to three minutes first, so wait for the application to go rather than
# for a fixed time.
$deadline = (Get-Date).AddMinutes(4)
while ((Test-Path $exe) -and ((Get-Date) -lt $deadline)) { Start-Sleep -Seconds 2 }
if (Test-Path $exe) { Fail "$exe still exists after uninstalling" }
if (Get-ItemProperty $uninstKey -ErrorAction SilentlyContinue) { Fail "the Apps & features entry is still there" }
if (-not $RemoveSettings) {
  $kept = Hashes
  foreach ($k in $before.Keys) {
    if (-not $kept.ContainsKey($k) -or $kept[$k] -ne $before[$k]) { Fail "$k did not survive the default uninstall" }
  }
  if (-not (CredentialPresent)) { Fail "the credential $credTarget did not survive the default uninstall" }
  Ok "uninstalled with the default: the configuration and the credential stay"
  # Clean up what this script wrote, and only that.
  Remove-Item -Force (Join-Path $confDir 'acceptance-desktop.yaml'), (Join-Path $confDir 'acceptance-installed.yaml'), (Join-Path $runnerHome 'acceptance-policy.yaml')
  cmdkey /delete:$credTarget | Out-Null
} else {
  Ok "uninstalled after the removal: nothing of Agentry Desktop remains"
}
Write-Host "PASS" -ForegroundColor Green
