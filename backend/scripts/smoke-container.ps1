#requires -Version 7.2
[CmdletBinding()]
param([string]$Image = 'bharatchat-backend:candidate')

$ErrorActionPreference = 'Stop'
$docker = (Get-Command docker -ErrorAction Stop).Source
$prefix = 'bharatchat-smoke-' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
$network = $prefix + '-net'
$postgres = $prefix + '-pg'
$redis = $prefix + '-redis'
$api = $prefix + '-api'
$containers = [System.Collections.Generic.List[string]]::new()
$networkCreated = $false
$password = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24))
$jwtSecret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))

function Invoke-DockerChecked {
    param([string[]]$Arguments)
    $output = & $docker @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Docker command failed: $($Arguments[0])" }
    return $output
}

try {
    $imageUser = Invoke-DockerChecked -Arguments @('image', 'inspect', '--format', '{{.Config.User}}', $Image)
    if ($imageUser -ne 'bharatchat') { throw 'Candidate must run as the non-root bharatchat user.' }
    Invoke-DockerChecked -Arguments @('network', 'create', '--internal', '--label', 'bharatchat.verification=true', $network) | Out-Null
    $networkCreated = $true
    $containers.Add($postgres)
    Invoke-DockerChecked -Arguments @('run', '--detach', '--name', $postgres, '--network', $network,
        '--label', 'bharatchat.verification=true', '--env', 'POSTGRES_USER=smoke',
        '--env', "POSTGRES_PASSWORD=$password", '--env', 'POSTGRES_DB=smoke', 'postgres:16-alpine') | Out-Null
    $containers.Add($redis)
    Invoke-DockerChecked -Arguments @('run', '--detach', '--name', $redis, '--network', $network,
        '--label', 'bharatchat.verification=true', 'redis:7-alpine') | Out-Null

    # Wait for the final PostgreSQL server, not the temporary initialization server.
    $databaseReady = $false
    for ($attempt = 0; $attempt -lt 90; $attempt++) {
        $logs = (& $docker logs $postgres 2>&1) -join "`n"
        if ([regex]::Matches($logs, 'database system is ready to accept connections').Count -ge 2) {
            $databaseReady = $true
            break
        }
        Start-Sleep -Seconds 1
    }
    if (-not $databaseReady) { throw 'Disposable PostgreSQL did not finish initialization.' }

    # Development mode exercises packaging/migrations, not production TLS or SMS.
    # No user data, host mounts, production secrets, or outbound network are used.
    $containers.Add($api)
    Invoke-DockerChecked -Arguments @('run', '--detach', '--name', $api, '--network', $network,
        '--label', 'bharatchat.verification=true', '--read-only', '--cap-drop', 'ALL',
        '--security-opt', 'no-new-privileges', '--tmpfs', '/tmp:rw,noexec,nosuid,size=16m',
        '--env', 'APP_ENV=development',
        '--env', "JWT_ACCESS_SECRET=$jwtSecret", '--env', "POSTGRES_HOST=$postgres",
        '--env', 'POSTGRES_USER=smoke', '--env', "POSTGRES_PASSWORD=$password",
        '--env', 'POSTGRES_DB=smoke', '--env', "REDIS_HOST=$redis",
        '--env', 'MESSAGING_ENABLED=false', '--env', 'OTP_DELIVERY_MODE=console', $Image) | Out-Null
    $baseURL = 'http://127.0.0.1:8080'
    $ready = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        $response = & $docker exec $api wget -q -T 3 -O - "$baseURL/readyz" 2>$null
        if ($LASTEXITCODE -eq 0 -and ($response | ConvertFrom-Json).status -eq 'ready') {
            $ready = $true
            break
        }
        $state = Invoke-DockerChecked -Arguments @('inspect', '--format', '{{.State.Status}}', $api)
        if ($state -ne 'running') { throw 'Candidate exited before becoming ready.' }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw 'Candidate failed its database/Redis readiness check.' }
    $health = Invoke-DockerChecked -Arguments @('exec', $api, 'wget', '-q', '-T', '3', '-O', '-', "$baseURL/healthz")
    if (($health | ConvertFrom-Json).status -ne 'ok') { throw 'Candidate failed liveness.' }
    $protected = (& $docker exec $api wget -S -T 3 -O /dev/null "$baseURL/api/v1/users/me" 2>&1) -join "`n"
    if ($LASTEXITCODE -eq 0 -or $protected -notmatch 'HTTP/1\.1 401\b') {
        throw 'Protected route did not reject an unauthenticated request.'
    }
    Write-Host 'Candidate startup passed: non-root/read-only, migrations, health 200, readiness 200, protected route 401.'
    Write-Host 'This is an isolated packaging smoke test, not a production deployment or live SMS test.'
}
finally {
    $containers.Reverse()
    foreach ($container in $containers) {
        & $docker container inspect $container *> $null
        if ($LASTEXITCODE -eq 0) { & $docker rm --force --volumes $container | Out-Null }
    }
    if ($networkCreated) { & $docker network rm $network | Out-Null }
}
