# Builds ModemPhone.
#
# Fyne needs cgo, and the only C compiler on this machine is the MSYS2 gcc in
# C:\msys64\ucrt64\bin, so the script puts that on PATH before invoking Go.
param(
    [switch]$Test,
    [switch]$Run
)

$ErrorActionPreference = "Continue"

# Go rejects a UTF-8 BOM, and some editors add one on save. Strip them first.
Get-ChildItem -Path $PSScriptRoot -Recurse -Filter *.go | ForEach-Object {
    $raw = [System.IO.File]::ReadAllBytes($_.FullName)
    if ($raw.Length -ge 3 -and $raw[0] -eq 0xEF -and $raw[1] -eq 0xBB -and $raw[2] -eq 0xBF) {
        [System.IO.File]::WriteAllBytes($_.FullName, $raw[3..($raw.Length - 1)])
        Write-Host "stripped BOM: $($_.Name)"
    }
}

$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
$env:CC = "gcc"

Push-Location $PSScriptRoot
try {
    cmd /c "gofmt -l ./cmd ./internal 2>&1"
    if ($Test) {
        cmd /c "go test ./... 2>&1"
        if ($LASTEXITCODE -ne 0) {
            Write-Host "TESTS FAILED"
            exit $LASTEXITCODE
        }
    }
    New-Item -ItemType Directory -Force -Path "$PSScriptRoot\bin" | Out-Null
    # -H windowsgui links against the GUI subsystem instead of the console one,
    # so launching the app does not flash a black terminal window behind it.
    # That also means nothing can be written to a console, which is why the app
    # keeps a log file instead (see internal/applog).
    cmd /c "go build -trimpath -ldflags ""-s -w -H windowsgui"" -o bin\modemphone.exe ./cmd/modemphone 2>&1"
    $code = $LASTEXITCODE
} finally {
    Pop-Location
}

if ($code -ne 0) {
    Write-Host "BUILD FAILED (exit $code)"
    exit $code
}

$exe = "$PSScriptRoot\bin\modemphone.exe"
$size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
Write-Host "BUILD OK -> bin\modemphone.exe ($size MB)"

if ($Run) {
    & $exe
}
exit 0
