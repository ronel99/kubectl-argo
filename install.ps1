#Requires -Version 5.1
$ErrorActionPreference = "Stop"

$Repo = "ronel99/kubectl-argo"
$BinaryName = "kubectl-argo.exe"
$InstallDir = if ($env:KUBECTL_ARGO_INSTALL_DIR) { $env:KUBECTL_ARGO_INSTALL_DIR } else { Join-Path $HOME ".kubectl-argo\bin" }
$Version = $env:KUBECTL_ARGO_VERSION

function Write-Info([string]$Message) {
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Success([string]$Message) {
    Write-Host "✓ $Message" -ForegroundColor Green
}

if (-not $Version) {
    Write-Info "Fetching latest release version from GitHub..."
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "kubectl-argo-installer" }
    $Version = $Release.tag_name
    if (-not $Version) {
        throw "Failed to resolve latest kubectl-argo version from GitHub API."
    }
}

$ArchiveName = "kubectl-argo_${Version}_windows_amd64.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Version"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("kubectl-argo-" + [System.Guid]::NewGuid().ToString("N"))

New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
try {
    $ZipPath = Join-Path $TempDir $ArchiveName
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"

    Write-Info "Downloading kubectl-argo $Version for windows/amd64..."
    Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName" -OutFile $ZipPath -UseBasicParsing

    try {
        Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath -UseBasicParsing
        $ExpectedLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($ArchiveName))$" } | Select-Object -First 1
        if ($ExpectedLine) {
            Write-Info "Verifying SHA-256 checksum..."
            $ExpectedHash = ($ExpectedLine -split "\s+")[0].ToLowerInvariant()
            $ActualHash = (Get-FileHash -Path $ZipPath -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($ExpectedHash -ne $ActualHash) {
                throw "Checksum mismatch for $ArchiveName! Expected $ExpectedHash, got $ActualHash."
            }
        }
    } catch {
        if ($_.Exception.Message -like "Checksum mismatch*") {
            throw $_
        }
    }

    Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $TargetBinary = Join-Path $InstallDir $BinaryName
    Copy-Item -Path (Join-Path $TempDir $BinaryName) -Destination $TargetBinary -Force

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathEntries = if ($UserPath) { $UserPath -split ";" } else { @() }
    if ($PathEntries -notcontains $InstallDir) {
        Write-Info "Adding $InstallDir to User PATH..."
        $NewUserPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    }
    if (($env:Path -split ";") -notcontains $InstallDir) {
        $env:Path = "$env:Path;$InstallDir"
    }

    Write-Success "Successfully installed kubectl-argo to $TargetBinary"
    & $TargetBinary version
} finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
