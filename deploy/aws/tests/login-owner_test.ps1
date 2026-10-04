$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$helper = Join-Path $PSScriptRoot '../login-owner.ps1'
$mock = Join-Path $PSScriptRoot 'login-aws.ps1'
$names = @('BHARATCHAT_LOGIN_TEST_ARN', 'BHARATCHAT_LOGIN_TEST_ACCOUNT', 'BHARATCHAT_LOGIN_TEST_FAIL')
$original = @{}
$previousExitCode = Get-Variable LASTEXITCODE -Scope Global -ValueOnly -ErrorAction SilentlyContinue
foreach ($name in $names) { $original[$name] = [Environment]::GetEnvironmentVariable($name) }
try {
    $env:BHARATCHAT_LOGIN_TEST_ACCOUNT = '123456789012'
    $env:BHARATCHAT_LOGIN_TEST_FAIL = '0'
    foreach ($arn in @(
        'arn:aws:iam::123456789012:root',
        'arn:aws:iam::123456789012:user/bharatchat/bharatchat-deployer',
        'arn:aws:iam::123456789012:user/bharatchat-deployer',
        'arn:aws:iam::999999999999:user/admin',
        'not-an-arn'
    )) {
        $env:BHARATCHAT_LOGIN_TEST_ARN = $arn
        $rejected = $false
        try { & $helper -ExpectedAccountId 123456789012 -AwsCLIPath $mock -VerifyOnly -NoPause } catch {
            if ($_.Exception.Message -notlike 'Wrong identity*') { throw }
            $rejected = $true
        }
        if (-not $rejected) { throw "Unsafe identity accepted: $arn" }
    }
    foreach ($arn in @(
        'arn:aws:iam::123456789012:user/admin',
        'arn:aws:sts::123456789012:assumed-role/AdministratorAccess/operator'
    )) {
        $env:BHARATCHAT_LOGIN_TEST_ARN = $arn
        & $helper -ExpectedAccountId 123456789012 -AwsCLIPath $mock -VerifyOnly -NoPause
    }
    $env:BHARATCHAT_LOGIN_TEST_ACCOUNT = '999999999999'
    $rejected = $false
    try { & $helper -ExpectedAccountId 123456789012 -AwsCLIPath $mock -VerifyOnly -NoPause } catch {
        if ($_.Exception.Message -notlike 'Wrong identity*') { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Mismatched account was accepted.' }
    $env:BHARATCHAT_LOGIN_TEST_FAIL = '1'
    $rejected = $false
    try { & $helper -ExpectedAccountId 123456789012 -AwsCLIPath $mock -VerifyOnly -NoPause } catch {
        if ($_.Exception.Message -notlike 'Administrator identity could not*') { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Failed STS lookup was accepted.' }
    Write-Host 'PASS: administrator helper rejects Root, deployer, wrong accounts, invalid identities and STS failures. No AWS calls.'
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $original[$name]) }
    $global:LASTEXITCODE = if ($null -eq $previousExitCode) { 0 } else { $previousExitCode }
}
