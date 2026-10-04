# Fake AWS CLI used only by create-host_test.ps1. Never forwards to AWS.
if ($env:BHARATCHAT_PROVISION_TEST -ne '1') { throw 'Test double must not be used outside the provisioner tests.' }
$global:LASTEXITCODE = 0
$operation = "$($args[0]) $($args[1])"
Add-Content -LiteralPath $env:BHARATCHAT_TEST_COMMAND_LOG -Value ($args -join ' ') -WhatIf:$false -Confirm:$false
switch ($operation) {
    'sts get-caller-identity' {
        if ($env:BHARATCHAT_TEST_CASE -eq 'root') {
            '{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:root"}'
        } elseif ($env:BHARATCHAT_TEST_CASE -eq 'wrong-account') {
            '{"Account":"999999999999","Arn":"arn:aws:iam::999999999999:user/test"}'
        } else { '{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:role/test"}' }
    }
    'ec2 describe-subnets' {
        '{"Subnets":[{"VpcId":"vpc-1234abcd","State":"available"}]}'
    }
    'ec2 describe-vpc-attribute' { '{"EnableDnsSupport":{"Value":true}}' }
    'ec2 describe-route-tables' {
        if ($env:BHARATCHAT_TEST_CASE -eq 'private-subnet') {
            '{"RouteTables":[{"Routes":[{"DestinationCidrBlock":"0.0.0.0/0","NatGatewayId":"nat-1234","State":"active"}]}]}'
        } elseif ($env:BHARATCHAT_TEST_CASE -eq 'main-route' -and ($args -contains 'Name=association.subnet-id,Values=subnet-1234abcd')) {
            '{"RouteTables":[]}'
        } else {
            '{"RouteTables":[{"Routes":[{"DestinationCidrBlock":"0.0.0.0/0","GatewayId":"igw-1234","State":"active"}]}]}'
        }
    }
    'cloudformation validate-template' { '{}' }
    'cloudformation create-stack' {
        if ($env:BHARATCHAT_TEST_CASE -eq 'existing-stack') { $global:LASTEXITCODE = 254; return }
        '{"StackId":"arn:aws:cloudformation:ap-south-1:123456789012:stack/test/abc"}'
    }
    'cloudformation wait' {
        if ($env:BHARATCHAT_TEST_CASE -in @('wait-failure', 'events-denied')) { $global:LASTEXITCODE = 255 }
    }
    'cloudformation describe-stack-events' {
        if ($env:BHARATCHAT_TEST_CASE -eq 'events-denied') { $global:LASTEXITCODE = 254; return }
        '{"StackEvents":[{"LogicalResourceId":"Host","ResourceStatus":"CREATE_FAILED","ResourceStatusReason":"Free Tier restriction"}]}'
    }
    'cloudformation describe-stacks' {
        $status = if ($env:BHARATCHAT_TEST_CASE -eq 'incomplete-stack') { 'CREATE_IN_PROGRESS' } else { 'CREATE_COMPLETE' }
        $protected = $env:BHARATCHAT_TEST_CASE -ne 'unprotected-stack'
        $outputs = @(
            @{OutputKey = 'InstanceId'; OutputValue = 'i-test'},
            @{OutputKey = 'ElasticIP'; OutputValue = '203.0.113.20'},
            @{OutputKey = 'AppSecurityGroupId'; OutputValue = 'sg-test'}
        )
        if ($env:BHARATCHAT_TEST_CASE -eq 'missing-output') { $outputs = @($outputs[0]) }
        @{Stacks = @(@{StackStatus = $status; EnableTerminationProtection = $protected; Outputs = $outputs})} | ConvertTo-Json -Depth 5
    }
    default { throw "Unexpected command in test: $operation" }
}
