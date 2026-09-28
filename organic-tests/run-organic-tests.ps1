# Seeded, isolated validation runner. It never deletes a prior lab run.
param(
    [Int64]$Seed = 0
)

$ErrorActionPreference = "Stop"
$runner = Join-Path $PSScriptRoot "runner"

Push-Location $runner
try {
    if ($Seed -eq 0) {
        go run .
    } else {
        go run . -seed $Seed
    }
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
