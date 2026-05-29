$ErrorActionPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Test-Url($u) {
    try {
        $r = Invoke-WebRequest -Uri $u -Method Head -UseBasicParsing -TimeoutSec 15
        return "OK $($r.StatusCode)"
    } catch {
        return "FAIL " + $_.Exception.Message
    }
}

"=== Connectivity ==="
"go.dev          : " + (Test-Url 'https://go.dev/dl/')
"dl.google(go)   : " + (Test-Url 'https://dl.google.com/go/go1.22.5.windows-amd64.msi')
"desktop.docker  : " + (Test-Url 'https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe')

"=== Latest Go version ==="
try {
    $j = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json&include=stable' -TimeoutSec 20
    $j[0].version
} catch {
    "GODEV JSON FAIL: " + $_.Exception.Message
}

"=== Current state ==="
"go on PATH     : " + ((Get-Command go -ErrorAction SilentlyContinue).Source)
"docker on PATH : " + ((Get-Command docker -ErrorAction SilentlyContinue).Source)
"=== Admin? ==="
$id = [Security.Principal.WindowsIdentity]::GetCurrent()
$p  = New-Object Security.Principal.WindowsPrincipal($id)
"IsAdmin: " + $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
