[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem

$Repository = 'Paaswn/yoel'
$DefaultReleaseApiUrl = "https://api.github.com/repos/$Repository/releases/latest"
$DefaultDownloadBaseUrl = "https://github.com/$Repository/releases/download"

function Fail([string]$Message) { throw "yoel installer: $Message" }

function Test-Version([string]$Version) {
    return $Version -match '^v[0-9][0-9A-Za-z._-]*$'
}

function Normalize-PathEntry([string]$Entry) {
    return $Entry.Trim().TrimEnd('\', '/')
}

function Test-PathContains([string]$PathValue, [string]$Directory) {
    $target = (Normalize-PathEntry $Directory)
    foreach ($entry in ($PathValue -split ';')) {
        if ((Normalize-PathEntry $entry).Equals($target, [StringComparison]::OrdinalIgnoreCase)) { return $true }
    }
    return $false
}

$architecture = [Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITEW6432')
if ([string]::IsNullOrWhiteSpace($architecture)) { $architecture = $env:PROCESSOR_ARCHITECTURE }
switch ($architecture.ToUpperInvariant()) {
    'AMD64' { $Arch = 'amd64' }
    'ARM64' { $Arch = 'arm64' }
    default { Fail "unsupported CPU architecture: $architecture" }
}

$releaseApiUrl = $DefaultReleaseApiUrl
$downloadBaseUrl = $DefaultDownloadBaseUrl
if ($env:YOEL_TEST_MODE -eq '1') {
    if ($env:YOEL_RELEASE_API_URL) { $releaseApiUrl = $env:YOEL_RELEASE_API_URL }
    if ($env:YOEL_RELEASE_DOWNLOAD_BASE_URL) { $downloadBaseUrl = $env:YOEL_RELEASE_DOWNLOAD_BASE_URL }
}

if ($env:YOEL_VERSION) {
    $version = $env:YOEL_VERSION
} else {
    try { $version = (Invoke-RestMethod -Uri $releaseApiUrl -UseBasicParsing).tag_name }
    catch { Fail "could not find the latest release; see https://github.com/$Repository/releases" }
}
if (-not (Test-Version $version)) { Fail "invalid release version; see https://github.com/$Repository/releases" }

$installDir = if ($env:YOEL_INSTALL_DIR) { $env:YOEL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\yoel\bin' }
if ([string]::IsNullOrWhiteSpace($installDir)) { Fail 'install directory is empty' }
$archive = "yoel_${version}_windows_${Arch}.zip"
$temporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("yoel-install-" + [Guid]::NewGuid().ToString('N'))

try {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    $archivePath = Join-Path $temporaryDirectory $archive
    $checksumsPath = Join-Path $temporaryDirectory 'checksums.txt'
    $downloadUrl = "$downloadBaseUrl/$version"
    try {
        Invoke-WebRequest -Uri "$downloadUrl/$archive" -OutFile $archivePath -UseBasicParsing
        Invoke-WebRequest -Uri "$downloadUrl/checksums.txt" -OutFile $checksumsPath -UseBasicParsing
    } catch { Fail "could not download release assets: $($_.Exception.Message)" }

    $checksumLine = Get-Content -LiteralPath $checksumsPath | Where-Object { $_ -match "^([0-9A-Fa-f]{64})\s+$([regex]::Escape($archive))$" } | Select-Object -First 1
    if (-not $checksumLine) { Fail "checksum for $archive is missing" }
    $expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { Fail "checksum verification failed for $archive" }

    try { $zip = [System.IO.Compression.ZipFile]::OpenRead($archivePath) }
    catch { Fail 'downloaded archive is invalid' }
    try {
        $entry = $zip.Entries | Where-Object { $_.FullName -eq 'yoel.exe' } | Select-Object -First 1
        if (-not $entry) { Fail 'archive does not contain the expected yoel.exe executable' }
        $extractedPath = Join-Path $temporaryDirectory 'yoel.exe'
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $extractedPath, $true)
    } finally { $zip.Dispose() }

    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $destination = Join-Path $installDir 'yoel.exe'
    $staging = Join-Path $installDir ('.yoel.new.' + [Guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath $extractedPath -Destination $staging
    try {
        if (Test-Path -LiteralPath $destination) {
            [System.IO.File]::Replace($staging, $destination, $null)
        } else {
            [System.IO.File]::Move($staging, $destination)
        }
    }
    catch { Remove-Item -LiteralPath $staging -Force -ErrorAction SilentlyContinue; Fail "could not replace $destination (it may be running): $($_.Exception.Message)" }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (Test-PathContains $userPath $installDir)) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { "$userPath;$installDir" }
        [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
        if (-not (Test-PathContains $env:Path $installDir)) { $env:Path = "$env:Path;$installDir" }
        Write-Output 'Added Yoel to your user PATH. Open a new terminal before using it there.'
    }

    Write-Output "Installed Yoel to $destination"
    Write-Output 'Verify the installation with: yoel --help'
} finally {
    if (Test-Path -LiteralPath $temporaryDirectory) { Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force }
}
