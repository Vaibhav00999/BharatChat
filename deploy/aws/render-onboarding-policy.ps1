[CmdletBinding()]
param([Parameter(Mandatory)][ValidatePattern('^[0-9]{12}$')][string]$AccountId)
$ErrorActionPreference = 'Stop'
$policy = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'deployer-onboarding-policy.json') -Raw | ConvertFrom-Json
foreach ($statement in $policy.Statement) {
    if ($statement.Resource -eq '*') { continue }
    # IAM variables are valid in ARN resource paths, not the ARN account field.
    $arn = $statement.Resource.Split(':', 6)
    if ($arn.Count -ne 6 -or $arn[0] -ne 'arn') { throw 'Invalid resource ARN in policy template.' }
    if ($arn[4] -eq 'ACCOUNT_ID') { $arn[4] = $AccountId }
    $statement.Resource = $arn -join ':'
}
$policy | ConvertTo-Json -Depth 20
