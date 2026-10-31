# hax installer for Windows (PowerShell 5.1 or newer).
#
# Usage:
#   irm https://raw.githubusercontent.com/BTCBap/hax/main/install.ps1 | iex
#   powershell -ExecutionPolicy Bypass -File install.ps1
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Uninstall
#
# Environment:
#   HAX_VERSION  release tag to install (for example v1.4.0)
#
# The script writes hax.exe to %LOCALAPPDATA%\Programs\hax, adds that folder
# to the user PATH when needed, and can create a Start Menu shortcut. It
# never reads or changes wallet data in %USERPROFILE%\.hax.

[CmdletBinding()]
param(
    [switch]$Uninstall,
    [switch]$Shortcut,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Repo = "BTCBap/hax"
$InstallDir = Join-Path -Path $env:LOCALAPPDATA -ChildPath "Programs\hax"
$Target = Join-Path -Path $InstallDir -ChildPath "hax.exe"

function Get-ShortcutPath {
    $programs = [Environment]::GetFolderPath("Programs")
    if ([string]::IsNullOrWhiteSpace($programs)) {
        $programs = Join-Path -Path $env:APPDATA -ChildPath "Microsoft\Windows\Start Menu\Programs"
    }
    return (Join-Path -Path $programs -ChildPath "hax.lnk")
}

function Write-Info {
    param([string]$Message)
    Write-Host $Message
}

function Stop-Install {
    param([string]$Message)
    throw "hax: $Message"
}

function Show-Usage {
    Write-Info @"
hax installer (Windows)

Usage:
  irm https://raw.githubusercontent.com/BTCBap/hax/main/install.ps1 | iex
  powershell -ExecutionPolicy Bypass -File install.ps1 [-Shortcut]
  powershell -ExecutionPolicy Bypass -File install.ps1 -Uninstall

Options:
  -Shortcut   Create a Start Menu shortcut
  -Uninstall  Remove hax.exe and the Start Menu shortcut
  -Help       Show this help

Environment:
  HAX_VERSION  Release to install, for example v1.4.0

The installer uses %LOCALAPPDATA%\Programs\hax and adds it to your user PATH.
Download first if you want to read the script, then run:
  powershell -ExecutionPolicy Bypass -File install.ps1
"@
}

function Get-UserPath {
    $value = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($null -eq $value) {
        return ""
    }
    return $value
}

function Test-PathEntry {
    param([string]$PathValue, [string]$Dir)
    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return $false
    }
    $wanted = $Dir.Trim().Trim([char]'"').TrimEnd([char]'\')
    foreach ($entry in ($PathValue -split ';')) {
        if ([string]::IsNullOrWhiteSpace($entry)) {
            continue
        }
        $candidate = $entry.Trim().Trim([char]'"').TrimEnd([char]'\')
        if ($candidate -ieq $wanted) {
            return $true
        }
    }
    return $false
}

function Add-ToUserPath {
    param([string]$Dir)
    $userPath = Get-UserPath
    $inSession = Test-PathEntry $env:Path $Dir

    if (-not (Test-PathEntry $userPath $Dir) -and -not $inSession) {
        if ([string]::IsNullOrWhiteSpace($userPath)) {
            $newPath = $Dir
        } else {
            $newPath = $userPath.TrimEnd([char]';') + ';' + $Dir
        }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Info "Added $Dir to the user PATH."
    }

    if (-not $inSession) {
        $env:Path = $env:Path.TrimEnd([char]';') + ';' + $Dir
    }
}

function Remove-FromUserPath {
    param([string]$Dir)
    $userPath = Get-UserPath
    if (Test-PathEntry $userPath $Dir) {
        $kept = @()
        foreach ($entry in ($userPath -split ';')) {
            if (-not [string]::IsNullOrWhiteSpace($entry) -and -not (Test-PathEntry $entry $Dir)) {
                $kept += $entry
            }
        }
        [Environment]::SetEnvironmentVariable("Path", ($kept -join ';'), "User")
        Write-Info "Removed $Dir from the user PATH."
    }

    if (Test-PathEntry $env:Path $Dir) {
        $kept = @()
        foreach ($entry in ($env:Path -split ';')) {
            if (-not [string]::IsNullOrWhiteSpace($entry) -and -not (Test-PathEntry $entry $Dir)) {
                $kept += $entry
            }
        }
        $env:Path = ($kept -join ';')
    }
}

function New-StartMenuShortcut {
    try {
        $shortcutPath = Get-ShortcutPath
        $parent = Split-Path -Parent $shortcutPath
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
        $wsh = New-Object -ComObject WScript.Shell
        $link = $wsh.CreateShortcut($shortcutPath)
        $link.TargetPath = $Target
        $link.WorkingDirectory = Split-Path -Parent $Target
        $link.Description = "Terminal Bitcoin wallet"
        $link.Save()
        Write-Info "Created Start Menu shortcut: $shortcutPath"
    } catch {
        Write-Warning "Could not create the Start Menu shortcut: $($_.Exception.Message)"
    }
}

function Install-Hax {
    # Windows PowerShell 5.1 needs TLS 1.2 for the GitHub download.
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch {
        # Newer PowerShell versions handle TLS themselves.
    }

    $Version = $env:HAX_VERSION
    if ([string]::IsNullOrWhiteSpace($Version)) {
        Write-Info "Looking up the latest hax release..."
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
            -Headers @{ "User-Agent" = "hax-install" } -UseBasicParsing
        $Version = $release.tag_name
    }
    if ([string]::IsNullOrWhiteSpace($Version)) {
        Stop-Install "could not find the latest release; set HAX_VERSION instead"
    }
    if ($Version -notmatch '^v') {
        $Version = "v$Version"
    }
    if ($Version -notmatch '^v[0-9][0-9A-Za-z._-]*$') {
        Stop-Install "invalid HAX_VERSION '$Version'; expected a tag such as v1.4.0"
    }

    $VersionNumber = $Version.Substring(1)
    $ArchiveName = "hax_${VersionNumber}_windows_amd64.zip"
    $BaseUrl = "https://github.com/$Repo/releases/download/$Version"

    $TempDir = Join-Path -Path ([IO.Path]::GetTempPath()) -ChildPath ("hax-install-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $TempDir | Out-Null

    try {
        $ArchivePath = Join-Path -Path $TempDir -ChildPath $ArchiveName
        $ChecksumsPath = Join-Path -Path $TempDir -ChildPath "checksums.txt"

        Write-Info "hax installer"
        Write-Info "  version:    $Version"
        Write-Info "  platform:   windows/amd64"
        Write-Info "  install to: $Target"
        Write-Info ""

        Write-Info "Downloading $ArchiveName..."
        Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName" -OutFile $ArchivePath -UseBasicParsing
        Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath -UseBasicParsing

        $expected = $null
        foreach ($line in (Get-Content -LiteralPath $ChecksumsPath)) {
            $parts = $line -split '\s+'
            if ($parts.Count -ge 2 -and $parts[1] -eq $ArchiveName) {
                $expected = $parts[0]
                break
            }
        }
        if ([string]::IsNullOrWhiteSpace($expected)) {
            Stop-Install "checksums.txt has no entry for $ArchiveName"
        }
        $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash
        if ($expected.ToUpperInvariant() -ne $actual.ToUpperInvariant()) {
            Stop-Install "checksum mismatch for $ArchiveName (expected $expected, got $actual)"
        }
        Write-Info "Checksum verified: $ArchiveName"

        $ExtractDir = Join-Path -Path $TempDir -ChildPath "extract"
        Expand-Archive -LiteralPath $ArchivePath -DestinationPath $ExtractDir -Force
        $ExtractedExe = Join-Path -Path $ExtractDir -ChildPath "hax.exe"
        if (-not (Test-Path -LiteralPath $ExtractedExe)) {
            Stop-Install "$ArchiveName does not contain hax.exe"
        }

        $oldVersion = $null
        if (Test-Path -LiteralPath $Target) {
            try {
                $oldVersion = (& $Target --version 2>$null | Select-Object -First 1)
            } catch {
                $oldVersion = $null
            }
        }

        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $tempExe = Join-Path -Path $InstallDir -ChildPath (".hax.tmp." + $PID + ".exe")
        Copy-Item -LiteralPath $ExtractedExe -Destination $tempExe -Force
        if (Test-Path -LiteralPath $Target) {
            Remove-Item -LiteralPath $Target -Force
        }
        Move-Item -LiteralPath $tempExe -Destination $Target -Force

        Add-ToUserPath $InstallDir

        if ($Shortcut) {
            New-StartMenuShortcut
        }

        if ($null -ne $oldVersion -and $oldVersion -match [regex]::Escape($Version)) {
            Write-Info "hax $Version is already up to date at $Target"
        } elseif ($null -ne $oldVersion -and -not [string]::IsNullOrWhiteSpace($oldVersion)) {
            Write-Info "Updated hax from $oldVersion to $Version at $Target"
        } else {
            Write-Info "Installed hax $Version to $Target"
        }

        Write-Info ""
        Write-Info "Installed version:"
        & $Target --version
        Write-Info ""
        Write-Info "Run hax by typing: hax"
        Write-Info "Open a new terminal if this one cannot find hax yet."
    } finally {
        Remove-Item -LiteralPath $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Uninstall-Hax {
    Write-Info "Removing the hax binary and Start Menu shortcut."
    Write-Info "Wallet data in $env:USERPROFILE\.hax is not touched."

    if (Test-Path -LiteralPath $Target) {
        Remove-Item -LiteralPath $Target -Force
        Write-Info "Removed $Target"
    }

    $shortcutPath = Get-ShortcutPath
    if (Test-Path -LiteralPath $shortcutPath) {
        Remove-Item -LiteralPath $shortcutPath -Force
        Write-Info "Removed $shortcutPath"
    }

    Remove-FromUserPath $InstallDir

    if (Test-Path -LiteralPath $InstallDir) {
        $remaining = @(Get-ChildItem -LiteralPath $InstallDir -Force -ErrorAction SilentlyContinue)
        if ($remaining.Count -eq 0) {
            Remove-Item -LiteralPath $InstallDir -Force
        }
    }

    Write-Info "Uninstall finished. Open a new terminal to refresh PATH."
}

if ($Help) {
    Show-Usage
    exit 0
}

if ($Uninstall) {
    Uninstall-Hax
    exit 0
}

Install-Hax
