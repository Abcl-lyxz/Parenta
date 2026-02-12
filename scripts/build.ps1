# Build script for Parenta (Windows PowerShell)
# Cross-compiles Go binary for OpenWrt ARM64 (Xiaomi AX3000T)

param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"

# Get the project root directory (parent of scripts folder)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

# Change to project root
Push-Location $ProjectRoot

try {
    $BuildDir = Join-Path $ProjectRoot "build"
    $BinaryName = "parenta"

    Write-Host "=== Building Parenta v$Version ===" -ForegroundColor Cyan
    Write-Host "Project root: $ProjectRoot"

    # Clean previous build
    if (Test-Path $BuildDir) {
        Remove-Item -Recurse -Force $BuildDir
    }
    New-Item -ItemType Directory -Path $BuildDir | Out-Null

    # Get dependencies
    Write-Host "Getting dependencies..."
    go mod tidy
    if ($LASTEXITCODE -ne 0) {
        throw "go mod tidy failed"
    }

    # Build for OpenWrt ARM64 (aarch64)
    Write-Host "Building for OpenWrt ARM64 (aarch64)..."

    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "arm64"

    $OutputBinary = Join-Path $BuildDir $BinaryName
    go build -trimpath -ldflags="-s -w -X main.Version=$Version" -o $OutputBinary ./cmd/parenta

    if ($LASTEXITCODE -ne 0) {
        throw "go build failed"
    }

    # Reset environment
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

    # Check if binary was created
    if (-not (Test-Path $OutputBinary)) {
        throw "Binary was not created at $OutputBinary"
    }

    # Show binary size
    Write-Host "Binary size:"
    Get-Item $OutputBinary | Select-Object Name, @{N='Size (MB)';E={[math]::Round($_.Length/1MB, 2)}}

    # Copy web assets
    Write-Host "Copying web assets..."
    Copy-Item -Recurse -Force (Join-Path $ProjectRoot "web") (Join-Path $BuildDir "web")

    # Copy config template
    $ConfigsDir = Join-Path $BuildDir "configs"
    New-Item -ItemType Directory -Path $ConfigsDir -Force | Out-Null
    Copy-Item (Join-Path $ProjectRoot "configs\parenta.json") $ConfigsDir

    # Copy deployment files
    Write-Host "Copying deployment files..."
    Copy-Item -Recurse -Force (Join-Path $ProjectRoot "deploy") (Join-Path $BuildDir "deploy")
    Copy-Item -Recurse -Force (Join-Path $ProjectRoot "scripts") (Join-Path $BuildDir "scripts")

    # Create deployment archive
    Write-Host "Creating deployment archive..."
    $ArchiveName = "parenta-$Version-openwrt-arm64.tar.gz"
    Push-Location $BuildDir
    tar -czvf $ArchiveName $BinaryName web configs deploy scripts
    Pop-Location

    Write-Host ""
    Write-Host "=== Build Complete ===" -ForegroundColor Green
    Write-Host ""
    Write-Host "Output: $BuildDir\$ArchiveName"
    Write-Host ""
    Write-Host "To deploy to router:" -ForegroundColor Yellow
    Write-Host "  1. scp $BuildDir\$ArchiveName root@router:/tmp/"
    Write-Host "  2. ssh root@router"
    Write-Host "  3. cd /tmp && tar -xzf $ArchiveName"
    Write-Host "  4. ./scripts/setup.sh"

} finally {
    Pop-Location
}
