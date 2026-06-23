# Invoked by the Azure DevOps agent on Windows (PowerShell Core). The SYSTEM_*
# and BUILD_* pipeline variables are already in the environment; OPENAI_API_KEY
# must be set as a secret pipeline variable and mapped in via the task's env block.
$ErrorActionPreference = 'Stop'

$taskDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$binary  = Join-Path $taskDir 'bin\windows-amd64\ai-code-review.exe'

if (-not (Test-Path $binary)) {
    Write-Host "##[error]Binary not found: $binary (build it with: make build-windows)"
    exit 1
}

# Map task inputs to the environment variables the binary reads.
$env:FAIL_ON     = $env:INPUT_FAILONSEVERITY
$env:CONFIG_PATH = if ($env:INPUT_CONFIGPATH) { $env:INPUT_CONFIGPATH } else { '.codereview.yml' }

& $binary
exit $LASTEXITCODE
