if (-not $env:BHARATCHAT_LOGIN_TEST_ARN) { throw 'This mock requires the login-owner test fixture.' }
if ($args.Count -lt 2 -or $args[0] -ne 'sts' -or $args[1] -ne 'get-caller-identity') {
    throw 'The verification helper must make only an STS identity request.'
}
$global:LASTEXITCODE = if ($env:BHARATCHAT_LOGIN_TEST_FAIL -eq '1') { 1 } else { 0 }
if ($global:LASTEXITCODE -eq 0) {
    @{ Account = $env:BHARATCHAT_LOGIN_TEST_ACCOUNT; Arn = $env:BHARATCHAT_LOGIN_TEST_ARN } | ConvertTo-Json -Compress
}
