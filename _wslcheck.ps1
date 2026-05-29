$ErrorActionPreference = 'SilentlyContinue'
"=== Windows optional features ==="
$wsl  = Get-WindowsOptionalFeature -Online -FeatureName Microsoft-Windows-Subsystem-Linux
$vmp  = Get-WindowsOptionalFeature -Online -FeatureName VirtualMachinePlatform
"WSL (Subsystem-Linux)   : " + $wsl.State
"VirtualMachinePlatform  : " + $vmp.State
"=== wsl.exe present? ==="
(Get-Command wsl.exe -ErrorAction SilentlyContinue).Source
"=== wsl version ==="
& wsl.exe --version 2>&1 | Out-String
