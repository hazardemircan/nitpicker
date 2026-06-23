# Run the reviewer locally against the diff between HEAD and a target branch.
#
# Defaults to MOCK_AI=1 and DRY_RUN=1, so it needs no API key and posts nothing.
# To exercise the real OpenAI call without posting to Azure DevOps:
#
#   $env:OPENAI_API_KEY = "sk-..."; $env:MOCK_AI = "0"; ./scripts/local-test.ps1
#
# Usage: ./scripts/local-test.ps1 [target-branch]   (default: main)
param([string]$TargetBranch = "main")

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path "$PSScriptRoot/..").Path

# Load a local .env if present (developer convenience; never used in CI).
$envFile = Join-Path $repoRoot ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        if ($_ -match '^\s*([^#=]+?)\s*=\s*(.*)$') {
            Set-Item "env:$($matches[1])" $matches[2].Trim()
        }
    }
}

if (-not (git -C $repoRoot rev-parse HEAD 2>$null)) {
    Write-Error "this repo has no commits yet"; exit 1
}

# Fall back to a local branch when origin/<branch> isn't available.
if (-not (git -C $repoRoot rev-parse "origin/$TargetBranch" 2>$null)) {
    if (git -C $repoRoot rev-parse $TargetBranch 2>$null) {
        Write-Host "note: origin/$TargetBranch not found, using local branch '$TargetBranch'"
        git -C $repoRoot update-ref "refs/remotes/origin/$TargetBranch" (git -C $repoRoot rev-parse $TargetBranch)
    } else {
        Write-Error "neither origin/$TargetBranch nor local branch '$TargetBranch' exists"; exit 1
    }
}

Write-Host "building..."
go build -o "$env:TEMP/ai-code-review.exe" $repoRoot

Write-Host "reviewing diff against $TargetBranch"
Write-Host "----------------------------------------"

$env:DRY_RUN = if ($env:DRY_RUN) { $env:DRY_RUN } else { "1" }
$env:MOCK_AI = if ($env:MOCK_AI) { $env:MOCK_AI } else { "1" }
$env:SYSTEM_PULLREQUEST_PULLREQUESTID = "999"
$env:SYSTEM_PULLREQUEST_TARGETBRANCHNAME = $TargetBranch
$env:BUILD_REPOSITORY_LOCALPATH = $repoRoot
$env:SYSTEM_TEAMFOUNDATIONCOLLECTIONURI = "https://dev.azure.com/local-test"
$env:SYSTEM_TEAMPROJECT = "local-test"
$env:BUILD_REPOSITORY_ID = "00000000-0000-0000-0000-000000000000"
$env:SYSTEM_ACCESSTOKEN = "dry-run"
if (-not $env:OPENAI_API_KEY) { $env:OPENAI_API_KEY = "dry-run" }

& "$env:TEMP/ai-code-review.exe"
