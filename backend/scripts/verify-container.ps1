#requires -Version 7.2
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$backend = Split-Path -Parent $PSScriptRoot
$docker = (Get-Command docker -ErrorAction Stop).Source
$image = 'bharatchat-backend:verification'
$results = Join-Path $backend 'test-results-container.jsonl'

Push-Location $backend
try {
    $engineOS = & $docker info --format '{{.OSType}}'
    if ($LASTEXITCODE -ne 0 -or $engineOS -ne 'linux') { throw 'A healthy Linux Docker engine is required.' }
    & $docker build --target test --tag $image .
    if ($LASTEXITCODE -ne 0) { throw 'Verification image build failed.' }
    if (Test-Path -LiteralPath $results -PathType Leaf) {
        Remove-Item -LiteralPath $results
    }

    # Only trusted local tests may use the host Docker socket. Testcontainers
    # creates disposable databases; no production credentials are supplied.
    & $docker run --rm `
        --mount 'type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock' `
        --mount 'type=volume,source=bharatchat-verification-go-cache,target=/root/.cache/go-build' `
        --add-host 'host.docker.internal:host-gateway' `
        --env 'TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal' `
        --env 'GOFLAGS=-buildvcs=false' --env 'GOMAXPROCS=2' `
        $image go test -json -race -p 1 -count=1 -timeout=15m ./... |
        Tee-Object -FilePath $results | Out-Null
    $testExit = $LASTEXITCODE
    if (-not (Test-Path -LiteralPath $results -PathType Leaf)) {
        throw "Test process exited with status $testExit without producing JSON evidence."
    }
    $events = @(Get-Content -LiteralPath $results | ForEach-Object { $_ | ConvertFrom-Json })
    $passed = @($events | Where-Object { $_.Test -and $_.Action -eq 'pass' })
    $skipped = @($events | Where-Object { $_.Test -and $_.Action -eq 'skip' })
    $failed = @($events | Where-Object { $_.Action -eq 'fail' })
    Write-Host "Tests passed: $($passed.Count); skipped: $($skipped.Count); failures: $($failed.Count)"
    Write-Host "Full evidence: $results"
    if ($testExit -ne 0 -or $failed.Count -or $skipped.Count -or -not $passed.Count) {
        throw 'Release verification failed; inspect the JSON test evidence.'
    }

    & $docker build --target production --tag 'bharatchat-backend:candidate' .
    if ($LASTEXITCODE -ne 0) { throw 'Production image build failed.' }
    & (Join-Path $PSScriptRoot 'smoke-container.ps1') -Image 'bharatchat-backend:candidate'
    Write-Host 'Backend race tests, production image build, and isolated startup passed. This does not approve public launch.'
}
finally {
    Pop-Location
}
