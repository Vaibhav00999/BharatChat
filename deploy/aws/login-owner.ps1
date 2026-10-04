[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^\d{12}$')][string]$ExpectedAccountId,
    [string]$AwsCLIPath,
    [switch]$VerifyOnly,
    [switch]$NoPause
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$aws = if ($AwsCLIPath) { $AwsCLIPath } else {
    Join-Path $env:LOCALAPPDATA 'Programs\Amazon\AWSCLIV2\aws.exe'
}
try {
    if (-not $VerifyOnly) {
        Write-Host 'ONE-TIME BHARATCHAT IAM SETUP'
        Write-Host 'Open the URL below in Chrome Incognito.'
        Write-Host 'Sign in as your separate administrator IAM user or role, NOT Root or bharatchat-deployer.'
        Write-Host 'Enter the authorization code only in this terminal, never in chat.'
        Write-Host 'Leave your normal Chrome window signed in as bharatchat-deployer.'
        & $aws login --remote --profile bharatchat-owner-bootstrap --region ap-south-1
        if ($LASTEXITCODE -ne 0) { throw 'Administrator sign-in did not complete.' }
    }
    $identity = & $aws sts get-caller-identity --profile bharatchat-owner-bootstrap --region ap-south-1 --output json --no-cli-pager
    if ($LASTEXITCODE -ne 0) { throw 'Administrator identity could not be verified.' }
    $caller = ($identity -join "`n") | ConvertFrom-Json
    if ($caller.Account -ne $ExpectedAccountId -or
        $caller.Arn -notmatch "^arn:aws:(iam|sts)::$($ExpectedAccountId):(user/.+|assumed-role/[^/]+/[^/]+)$" -or
        $caller.Arn -match ':user/(?:.*/)?bharatchat-deployer$') {
        throw "Wrong identity for administrator setup: $($caller.Arn). Root and the deployer are not accepted. No permissions have been changed."
    }
    Write-Host "Separate IAM identity verified: $($caller.Arn)"
    Write-Host 'Administrator permissions still require checking. No resources or policies were changed by this helper.'
} finally {
    if (-not $NoPause) { $null = Read-Host 'Press Enter to close and return to Codex' }
}
