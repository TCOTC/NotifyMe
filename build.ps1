# NotifyMe Build Script
# For building Windows executable

param(
    [switch]$Clean,
    [switch]$Debug,
    [switch]$Compress,
    [switch]$SkipFrontend
)

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 | Out-Null

$ErrorActionPreference = "Stop"

function Write-ColorOutput($ForegroundColor, $Message) {
    $fc = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    Write-Host $Message
    $host.UI.RawUI.ForegroundColor = $fc
}

function Write-Success($message) {
    Write-ColorOutput Green $message
}

function Write-Error-Custom($message) {
    Write-ColorOutput Red $message
}

function Write-Info($message) {
    Write-ColorOutput Cyan $message
}

function Test-Command($cmdname) {
    $null = Get-Command $cmdname -ErrorAction SilentlyContinue
    return $?
}

Write-Info "========================================="
Write-Info "    NotifyMe Build Script"
Write-Info "========================================="
Write-Host ""

Write-Info "[1/5] Checking build environment..."
if (-not (Test-Command "go")) {
    Write-Error-Custom "Error: Go not found, please install Go 1.25.0 or higher"
    exit 1
}
$goVersion = go version
Write-Success "[OK] Go: $goVersion"

if (-not (Test-Command "pnpm")) {
    Write-Error-Custom "Error: pnpm not found, please install pnpm"
    Write-Info "Install command: npm install -g pnpm"
    exit 1
}
$pnpmVersion = pnpm --version
Write-Success "[OK] pnpm: v$pnpmVersion"

if (-not (Test-Command "wails")) {
    Write-Error-Custom "Error: Wails CLI not found, please install"
    Write-Info "Install command: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
}
$wailsVersion = wails version
Write-Success "[OK] Wails: $wailsVersion"
Write-Host ""

if ($Clean) {
    Write-Info "[2/6] Cleaning build directories..."
    if (Test-Path "build\bin") {
        Remove-Item -Recurse -Force "build\bin"
        Write-Success "[OK] Cleaned build\bin directory"
    }
    if (Test-Path "frontend\dist") {
        Remove-Item -Recurse -Force "frontend\dist"
        Write-Success "[OK] Cleaned frontend\dist directory"
    }
    Write-Host ""
}

Write-Info "[2/6] Syncing icon file..."
if (Test-Path "build\windows\icon.ico") {
    Copy-Item -Path "build\windows\icon.ico" -Destination "internal\tray\icon.ico" -Force
    Write-Success "[OK] Icon file synced"
} else {
    Write-Info "[WARN] build\windows\icon.ico not found, skipping icon sync"
}
Write-Host ""

Write-Info "[3/6] Installing Go dependencies..."
try {
    go mod download 2>&1 | Out-Null
    go mod tidy 2>&1 | Out-Null
    Write-Success "[OK] Go dependencies installed"
} catch {
    Write-Error-Custom "Error: Failed to install Go dependencies"
    Write-Error-Custom $_.Exception.Message
    exit 1
}
Write-Host ""

if (-not $SkipFrontend) {
    Write-Info "[4/6] Installing frontend dependencies..."
    Push-Location frontend
    try {
        pnpm install 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "pnpm install failed with exit code: $LASTEXITCODE"
        }
        Write-Success "[OK] Frontend dependencies installed"
    } catch {
        Write-Error-Custom "Error: Failed to install frontend dependencies"
        Write-Error-Custom $_.Exception.Message
        Pop-Location
        exit 1
    }
    Pop-Location
    Write-Host ""
} else {
    Write-Info "[4/6] Skipping frontend dependencies installation"
    Write-Host ""
}

Write-Info "[5/6] Building application..."
$buildArgs = @("build")

if ($Debug) {
    $buildArgs += "-debug"
    Write-Info "Build mode: Debug"
}

if ($Compress) {
    $buildArgs += "-upx"
    Write-Info "Compression: Enabled (UPX)"
}

if ($SkipFrontend) {
    $buildArgs += "-s"
    Write-Info "Frontend build: Skipped"
}

if ($Clean) {
    $buildArgs += "-clean"
}

$buildCommand = "wails " + ($buildArgs -join " ")
Write-Info "Executing: $buildCommand"
Write-Host ""

$buildSuccess = $false
try {
    & wails $buildArgs
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) {
        $buildSuccess = $true
    } else {
        Write-Error-Custom "Error: Build failed with exit code: $exitCode"
    }
} catch {
    Write-Error-Custom "Error: Exception occurred during build"
    Write-Error-Custom $_.Exception.Message
    exit 1
}

Write-Host ""

$exePath = "build\bin\NotifyMe.exe"
if ($buildSuccess -and (Test-Path $exePath)) {
    $fileInfo = Get-Item $exePath
    $fileSize = [math]::Round($fileInfo.Length / 1MB, 2)
    
    Write-Info "========================================="
    Write-Success "Build Successful!"
    Write-Info "========================================="
    Write-Host ""
    Write-Info "Output file: $exePath"
    Write-Info "File size: $fileSize MB"
    Write-Info "Build time: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
    Write-Host ""
    Write-Success "You can run the program with:"
    Write-Host "  .\build\bin\NotifyMe.exe"
    Write-Host ""
} else {
    if (-not $buildSuccess) {
        Write-Error-Custom "Error: Build process failed"
    } else {
        Write-Error-Custom "Error: Output file not found: $exePath"
        Write-Info "Please check the build logs for more information"
    }
    exit 1
}
