[CmdletBinding(SupportsShouldProcess, ConfirmImpact = 'Medium')]
param(
    [string]$Profile = 'bharatchat-mumbai-prod',
    [string]$AwsCLIPath,
    [Parameter(Mandatory)][ValidatePattern('^\d{12}$')][string]$ExpectedAccountId,
    [Parameter(Mandatory)][ValidatePattern('^vpc-[a-f0-9]+$')][string]$VpcId,
    [Parameter(Mandatory)][ValidatePattern('^subnet-[a-f0-9]+$')][string]$PublicSubnetId,
    [Parameter(Mandatory)][string]$TesterIPv4,
    [Parameter(Mandatory)][ValidatePattern('^ami-[a-f0-9]+$')][string]$UbuntuImage,
    [ValidatePattern('^[a-zA-Z][a-zA-Z0-9-]{0,127}$')][string]$StackName = 'bharatchat-prelaunch',
    [ValidateSet('t3.medium', 't3.large')][string]$InstanceType = 't3.medium'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$region = 'ap-south-1'
$ip = $null
if ($TesterIPv4 -notmatch '^([0-9]{1,3}\.){3}[0-9]{1,3}/32$' -or
    -not [Net.IPAddress]::TryParse($TesterIPv4.Replace('/32', ''), [ref]$ip) -or
    "$($ip.ToString())/32" -ne $TesterIPv4 -or $ip.GetAddressBytes()[0] -in @(0, 127) -or
    $ip.GetAddressBytes()[0] -ge 224) { throw 'Use one canonical tester IPv4 address with /32.' }
$aws = if ($AwsCLIPath) {
    Get-Command $AwsCLIPath -CommandType Application,ExternalScript -ErrorAction Stop
} else {
    Get-Command aws -CommandType Application,ExternalScript -ErrorAction SilentlyContinue
}
if (-not $aws -and $env:LOCALAPPDATA) {
    $local = Join-Path $env:LOCALAPPDATA 'Programs\Amazon\AWSCLIV2\aws.exe'
    $aws = Get-Command $local -ErrorAction SilentlyContinue
}
if (-not $aws) { throw 'Install the official AWS CLI v2 first.' }
function Invoke-AwsJSON([string[]]$Arguments) {
    $result = & $aws.Source @Arguments --profile $Profile --region $region --output json --no-cli-pager
    if ($LASTEXITCODE -ne 0) { throw "AWS command failed ($($Arguments[0]) $($Arguments[1])); no later steps were run." }
    if ($result) { return ($result -join "`n" | ConvertFrom-Json) }
}
$caller = Invoke-AwsJSON @('sts', 'get-caller-identity')
if ($caller.Account -ne $ExpectedAccountId -or $caller.Arn -match ':root$') {
    throw 'Wrong AWS account or root identity. Use the intended IAM deployment role.'
}
$subnetResult = Invoke-AwsJSON @('ec2', 'describe-subnets', '--subnet-ids', $PublicSubnetId)
$subnet = @($subnetResult.Subnets)[0]
if ($subnet.VpcId -ne $VpcId -or $subnet.State -ne 'available') { throw 'Subnet must be available in the selected VPC.' }
$dns = Invoke-AwsJSON @('ec2', 'describe-vpc-attribute', '--vpc-id', $VpcId, '--attribute', 'enableDnsSupport')
if (-not $dns.EnableDnsSupport.Value) { throw 'VPC DNS support must be enabled.' }
$tableResult = Invoke-AwsJSON @('ec2', 'describe-route-tables', '--filters', "Name=association.subnet-id,Values=$PublicSubnetId")
$tables = @($tableResult.RouteTables)
if ($tables.Count -eq 0) {
    $tableResult = Invoke-AwsJSON @('ec2', 'describe-route-tables', '--filters', "Name=vpc-id,Values=$VpcId", 'Name=association.main,Values=true')
    $tables = @($tableResult.RouteTables)
}
if ($tables.Count -eq 0) { throw 'Subnet route table could not be resolved.' }
$publicRoute = @($tables.Routes | Where-Object {
    $_.PSObject.Properties.Name -contains 'DestinationCidrBlock' -and $_.DestinationCidrBlock -eq '0.0.0.0/0' -and
    $_.PSObject.Properties.Name -contains 'GatewayId' -and $_.GatewayId -like 'igw-*' -and $_.State -eq 'active'
})
if ($publicRoute.Count -eq 0) { throw 'Subnet needs an active IPv4 default route to an Internet Gateway.' }

$template = Join-Path $PSScriptRoot 'ec2-host.yml'
$null = Invoke-AwsJSON @('cloudformation', 'validate-template', '--template-body', "file://$template")
if (-not $PSCmdlet.ShouldProcess("$ExpectedAccountId / $region / $StackName", 'Create billable EC2, encrypted EBS, Elastic IP and restricted security group using a pre-created IAM role')) { return }

# create-stack refuses to overwrite an existing stack. Never delete/replace it here.
$parameters = @(
    "ParameterKey=VpcId,ParameterValue=$VpcId",
    "ParameterKey=PublicSubnetId,ParameterValue=$PublicSubnetId",
    "ParameterKey=TesterIPv4,ParameterValue=$TesterIPv4",
    "ParameterKey=UbuntuImage,ParameterValue=$UbuntuImage",
    "ParameterKey=InstanceType,ParameterValue=$InstanceType"
)
$created = Invoke-AwsJSON (@('cloudformation', 'create-stack', '--stack-name', $StackName,
    '--template-body', "file://$template",
    '--enable-termination-protection', '--parameters') + $parameters)
$runtime = Join-Path $PSScriptRoot 'runtime'
$null = New-Item -ItemType Directory -Path $runtime -Force
$attemptPath = Join-Path $runtime ("host-attempt-$([guid]::NewGuid().ToString('N')).json")
$attempt = [ordered]@{
    AccountId = $caller.Account; Region = $region; Profile = $Profile
    StackId = $created.StackId; Status = 'SUBMITTED'
    VpcId = $VpcId; PublicSubnetId = $PublicSubnetId; UbuntuImage = $UbuntuImage
    InstanceType = $InstanceType; SubmittedAt = [DateTime]::UtcNow.ToString('o')
}
$attempt | ConvertTo-Json | Set-Content -LiteralPath $attemptPath -Encoding utf8
Write-Host "Stack submitted: $($created.StackId). Waiting for AWS; failed creation requires inspection, not a blind retry."
try {
    $null = Invoke-AwsJSON @('cloudformation', 'wait', 'stack-create-complete', '--stack-name', $created.StackId)
    $description = Invoke-AwsJSON @('cloudformation', 'describe-stacks', '--stack-name', $created.StackId)
    if ($description.Stacks[0].StackStatus -ne 'CREATE_COMPLETE' -or
        -not $description.Stacks[0].EnableTerminationProtection) {
        throw 'Expected a completed, termination-protected host stack.'
    }
    $outputs = @{}
    foreach ($entry in $description.Stacks[0].Outputs) { $outputs[$entry.OutputKey] = $entry.OutputValue }
    foreach ($required in @('InstanceId', 'ElasticIP', 'AppSecurityGroupId')) {
        if ([string]::IsNullOrWhiteSpace($outputs[$required])) { throw "Missing required host output: $required" }
    }
} catch {
    $failure = $_
    $attempt.Status = 'VERIFICATION_FAILED'
    $attempt.Error = $failure.Exception.Message
    # Preserve evidence without changing, deleting, or retrying the failed stack.
    try {
        $events = Invoke-AwsJSON @('cloudformation', 'describe-stack-events', '--stack-name', $created.StackId)
        $eventsPath = [IO.Path]::ChangeExtension($attemptPath, 'events.json')
        $events | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $eventsPath -Encoding utf8
        $attempt.EventsPath = [IO.Path]::GetFileName($eventsPath)
    } catch { Write-Warning 'Could not retrieve stack events; inspect the recorded stack ID in AWS.' }
    $attempt | ConvertTo-Json | Set-Content -LiteralPath $attemptPath -Encoding utf8
    Write-Warning "Host verification failed. Evidence: $attemptPath. No automatic retry or deletion performed."
    throw $failure
}
$attempt.Status = 'CREATE_COMPLETE'
$attempt | ConvertTo-Json | Set-Content -LiteralPath $attemptPath -Encoding utf8
$record = [ordered]@{
    AccountId = $caller.Account; Region = $region; Profile = $Profile; StackId = $created.StackId
    InstanceId = $outputs.InstanceId; ElasticIP = $outputs.ElasticIP
    AppSecurityGroupId = $outputs.AppSecurityGroupId; TesterIPv4 = $TesterIPv4
    CreatedAt = [DateTime]::UtcNow.ToString('o')
}
$record | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runtime 'host.json') -Encoding utf8
Write-Host "Instance: $($record.InstanceId); Elastic IP: $($record.ElasticIP)"
Write-Host 'Recorded runtime/host.json. No DNS changes or backend deployment have been performed.'
