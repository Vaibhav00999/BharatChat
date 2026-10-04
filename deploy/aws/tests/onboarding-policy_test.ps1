$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$renderer = Join-Path $PSScriptRoot '../render-onboarding-policy.ps1'
$template = Join-Path $PSScriptRoot '../deployer-onboarding-policy.json'
$original = Get-Content -LiteralPath $template -Raw
$json = & $renderer -AccountId '123456789012'
$policy = $json | ConvertFrom-Json
if ($policy.Version -ne '2012-10-17' -or $json -match 'ACCOUNT_ID|\$\{aws:PrincipalAccount\}') {
    throw 'Policy account placeholders were not resolved.'
}
$self = $policy.Statement | Where-Object Sid -eq 'EnrollOwnMFAAndChangeOwnPassword'
if ($self.Resource -cne 'arn:aws:iam::123456789012:user/bharatchat/${aws:username}') {
    throw 'Rendering lost the self-only user scope.'
}
$mfa = $policy.Statement | Where-Object Sid -eq 'CreateOwnVirtualMFA'
if ($mfa.Resource -cne 'arn:aws:iam::123456789012:mfa/${aws:username}*') { throw 'Invalid MFA scope.' }
$network = $policy.Statement | Where-Object Sid -eq 'ReadMumbaiNetworkPreflight'
if ($network.Condition.StringEquals.'aws:RequestedRegion' -ne 'ap-south-1') { throw 'Missing region restriction.' }
$denyKeys = $policy.Statement | Where-Object Sid -eq 'DoNotCreateLongLivedAPICredentials'
if ($denyKeys.Effect -ne 'Deny' -or $denyKeys.Action -notcontains 'iam:CreateAccessKey') {
    throw 'API key creation must be explicitly denied.'
}
$allowed = @($policy.Statement | Where-Object Effect -eq 'Allow' | ForEach-Object Action)
$permitted = @(
    'iam:GetAccountPasswordPolicy', 'iam:ListVirtualMFADevices', 'iam:GetUser',
    'iam:ListMFADevices', 'iam:EnableMFADevice', 'iam:ResyncMFADevice', 'iam:ChangePassword',
    'iam:CreateVirtualMFADevice', 'ec2:DescribeVpcs', 'ec2:DescribeVpcAttribute',
    'ec2:DescribeSubnets', 'ec2:DescribeRouteTables', 'ec2:DescribeSecurityGroups',
    'ec2:DescribeInstances', 'ec2:DescribeAddresses', 'ec2:DescribeImages',
    'cloudformation:ValidateTemplate', 'cloudformation:DescribeStacks',
    'cloudformation:DescribeStackEvents', 'cloudformation:DescribeStackResources',
    'cloudformation:GetTemplate', 'ssm:GetParameter', 'ssm:GetParameters'
)
foreach ($action in $allowed) {
    if ($permitted -notcontains $action) { throw "Unexpected onboarding permission: $action" }
}
foreach ($account in @('', '123', '1234567890123', '12345678901*')) {
    $rejected = $false
    try { $null = & $renderer -AccountId $account } catch { $rejected = $true }
    if (-not $rejected) { throw 'Invalid account ID was accepted.' }
}
if ((Get-Content -LiteralPath $template -Raw) -cne $original) { throw 'Renderer changed the source template.' }
Write-Host 'PASS: onboarding policy renders an explicit account, preserves self/region scopes, and rejects invalid account IDs. No AWS calls.'
