$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$dst = 'C:\Temp\devinstall'
New-Item -ItemType Directory -Force -Path $dst | Out-Null

$goUrl  = 'https://dl.google.com/go/go1.26.3.windows-amd64.msi'
$goFile = Join-Path $dst 'go1.26.3.windows-amd64.msi'
$dkUrl  = 'https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe'
$dkFile = Join-Path $dst 'DockerDesktopInstaller.exe'

function Get-File($url, $out, $name) {
    if ((Test-Path $out) -and ((Get-Item $out).Length -gt 1MB)) {
        "$name : already downloaded ($([math]::Round((Get-Item $out).Length/1MB)) MB), skip"
        return
    }
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Invoke-WebRequest -Uri $url -OutFile $out -UseBasicParsing -TimeoutSec 1800
    $sw.Stop()
    "$name : downloaded $([math]::Round((Get-Item $out).Length/1MB)) MB in $([math]::Round($sw.Elapsed.TotalSeconds))s"
}

Get-File $goUrl $goFile 'Go MSI'
Get-File $dkUrl $dkFile 'Docker Desktop'
"DONE"
