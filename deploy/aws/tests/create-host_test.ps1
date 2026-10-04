$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('bharatchat-provision-test-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $temporary
$names = @('BHARATCHAT_PROVISION_TEST', 'BHARATCHAT_TEST_CASE', 'BHARATCHAT_TEST_COMMAND_LOG')
$original = @{}
$originalExitCode = Get-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue
$originalExitCodeValue = if ($null -ne $originalExitCode) { $originalExitCode.Value } else { $null }
foreach ($name in $names) { $original[$name] = [Environment]::GetEnvironmentVariable($name) }
try {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '../create-host.ps1') -Destination $temporary
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '../ec2-host.yml') -Destination $temporary
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'aws.ps1') -Destination $temporary
    $env:BHARATCHAT_PROVISION_TEST = '1'
    $env:BHARATCHAT_TEST_COMMAND_LOG = Join-Path $temporary 'commands.log'
    $script = Join-Path $temporary 'create-host.ps1'
    $parameters = @{
        ExpectedAccountId = '123456789012'; VpcId = 'vpc-1234abcd'
        PublicSubnetId = 'subnet-1234abcd'; TesterIPv4 = '203.0.113.10/32'
        UbuntuImage = 'ami-1234abcd'
        AwsCLIPath = Join-Path $temporary 'aws.ps1'
    }
    foreach ($case in @('root', 'wrong-account', 'private-subnet', 'existing-stack')) {
        $env:BHARATCHAT_TEST_CASE = $case
        Set-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Value ''
        $failed = $false
        try { & $script @parameters -Confirm:$false } catch {
            $failed = $true
            $expected = switch ($case) {
                'root' { 'Wrong AWS account or root identity' }
                'wrong-account' { 'Wrong AWS account' }
                'private-subnet' { 'Subnet needs an active IPv4 default route' }
                'existing-stack' { 'AWS command failed' }
            }
            if ($_.Exception.Message -notlike "*$expected*") { throw }
        }
        if (-not $failed) { throw "Expected rejection: $case" }
        $log = Get-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Raw
        if ($case -ne 'existing-stack' -and $log -match 'create-stack') { throw 'Preflight failure created a stack.' }
        if ($log -match 'cloudformation wait') { throw 'Failed creation continued into wait.' }
    }
    foreach ($case in @('wait-failure', 'events-denied', 'incomplete-stack', 'unprotected-stack', 'missing-output')) {
        $env:BHARATCHAT_TEST_CASE = $case
        Set-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Value ''
        $failed = $false
        try { & $script @parameters -Confirm:$false } catch {
            $failed = $true
            if ($_.Exception.Message -notmatch 'AWS command failed|Expected a completed|Missing required host output') { throw }
        }
        if (-not $failed) { throw "Expected rejection: $case" }
        if (Test-Path -LiteralPath (Join-Path $temporary 'runtime/host.json')) { throw 'Failed verification recorded a successful host.' }
        $attempts = @(Get-ChildItem -LiteralPath (Join-Path $temporary 'runtime') -Filter 'host-attempt-*.json' |
            Where-Object Name -NotLike '*.events.json')
        if ($attempts.Count -eq 0) { throw 'Submitted stack ID was not preserved.' }
        $latest = $attempts | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        $evidence = Get-Content -LiteralPath $latest.FullName -Raw | ConvertFrom-Json
        if ($evidence.StackId -notlike '*:stack/test/abc') { throw 'Incorrect attempt stack ID.' }
        if ($evidence.Status -ne 'VERIFICATION_FAILED') { throw 'Failure status was not preserved.' }
        $log = Get-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Raw
        if ($log -match 'delete-stack|update-stack|update-termination-protection') { throw 'Failure triggered a destructive change.' }
    }
    $env:BHARATCHAT_TEST_CASE = 'main-route'
    Set-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Value ''
    $attemptCount = @(Get-ChildItem -LiteralPath (Join-Path $temporary 'runtime') -Filter 'host-attempt-*.json').Count
    & $script @parameters -WhatIf
    $log = Get-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Raw
    if ($log -match 'create-stack') { throw '-WhatIf created a stack.' }
    if (@(Get-ChildItem -LiteralPath (Join-Path $temporary 'runtime') -Filter 'host-attempt-*.json').Count -ne $attemptCount) {
        throw '-WhatIf wrote an attempt record.'
    }
    & $script @parameters -Confirm:$false
    $log = Get-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Raw
    if ($log -notmatch '--enable-termination-protection' -or $log -notmatch '--profile bharatchat-mumbai-prod --region ap-south-1') {
        throw 'Expected provisioning safeguards were missing.'
    }
    if ($log -match 'CAPABILITY_IAM|CAPABILITY_NAMED_IAM|--role-arn' -or $log -notmatch 'ParameterKey=UbuntuImage,ParameterValue=ami-1234abcd') {
        throw 'Host provisioning must pin the image and must not request IAM creation or a service role.'
    }
    $record = Get-Content -LiteralPath (Join-Path $temporary 'runtime/host.json') -Raw | ConvertFrom-Json
    if ($record.InstanceId -ne 'i-test' -or $record.ElasticIP -ne '203.0.113.20') { throw 'Host output was not recorded.' }
    Write-Host 'PASS: mocked AWS identity/network guards, failed verification evidence, no destructive retries, WhatIf and host outputs. No real AWS calls.'
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $original[$name]) }
    if ($null -eq $originalExitCode) {
        Remove-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue
    } else { $global:LASTEXITCODE = $originalExitCodeValue }
    # Delete only the known generated fixtures, without recursive removal.
    $runtime = Join-Path $temporary 'runtime'
    if (Test-Path -LiteralPath $runtime) {
        Get-ChildItem -LiteralPath $runtime -Filter 'host-attempt-*.json' -File |
            Remove-Item -Force
    }
    foreach ($file in @('runtime/host.json', 'commands.log', 'aws.ps1', 'ec2-host.yml', 'create-host.ps1')) {
        Remove-Item -LiteralPath (Join-Path $temporary $file) -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath (Join-Path $temporary 'runtime')) { [IO.Directory]::Delete((Join-Path $temporary 'runtime')) }
    [IO.Directory]::Delete($temporary)
}
