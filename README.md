# AI Code Review for Azure DevOps

A small Go tool that reviews pull request diffs with OpenAI and posts the
findings back as inline comments on the PR. It runs as a step in an Azure
DevOps pipeline and can optionally fail the build when a serious issue is found.

## How it works

1. On a PR build, it computes the diff between the source and target branch.
2. Each changed file's diff is sent to the OpenAI Chat Completions API.
3. The model returns findings (line, severity, message) as JSON.
4. Each finding is posted as an inline comment thread on the pull request.
5. If any finding meets the configured severity, the step exits non-zero.

Findings have one of four severities: `blocker`, `major`, `minor`, `info`.

## Requirements

- Go 1.22+ (to build)
- An OpenAI API key
- An Azure DevOps pipeline with PR triggers

## Configuration

Review behavior is controlled by a `.codereview.yml` file at the root of the
repository being reviewed. All fields are optional; sensible defaults are used
when the file is absent — the tool works with zero configuration.

If the file is **present but malformed** (e.g. a YAML indentation error), the
step fails fast with a non-zero exit instead of silently falling back to
defaults. This is deliberate: degrading to defaults could quietly drop your
`failOn` setting and let a broken gate report success.

```yaml
version: 1

# Fail the build on findings at this severity or higher (blocker > major > minor > info).
# Use "none" to report only and never fail.
failOn: blocker

openAIModel: gpt-4o
maxFilesPerReview: 20

# Lines of unchanged context around each change (git --unified). Default 3.
diffContext: 3

excludePatterns:
  - "**/*_test.go"
  - "**/vendor/**"

rules:
  - "Check all error return values; do not discard errors with _"
  - "Never log secrets, tokens, or API keys"
```

See [.codereview.yml](.codereview.yml) for a fuller example.

## Local testing

You can run the reviewer against your local diff without touching Azure DevOps.
The helper scripts default to `MOCK_AI=1` and `DRY_RUN=1`, so no API key is
needed and nothing is posted.

```bash
# Linux / macOS
./scripts/local-test.sh                 # mock review of HEAD vs main

OPENAI_API_KEY=sk-... MOCK_AI=0 \
  ./scripts/local-test.sh               # real OpenAI call, still dry-run
```

```powershell
# Windows
./scripts/local-test.ps1                # mock review of HEAD vs main

$env:OPENAI_API_KEY = "sk-..."; $env:MOCK_AI = "0"
./scripts/local-test.ps1                # real OpenAI call, still dry-run
```

## Using it in Azure DevOps

First, add your key as a **secret pipeline variable** named `OPENAI_API_KEY`
(Pipeline → Edit → Variables → check "Keep this value secret"). Never put the
key in YAML.

The pipeline identity also needs permission to comment on PRs: enable
`persistCredentials: true` on checkout, map `System.AccessToken` into the step,
and grant the build service "Contribute to pull requests" on the repository.

### Option A — inline step (no extension to publish)

Build and run the binary directly in your pipeline:

```yaml
pr:
  branches: { include: [ main ] }

pool:
  vmImage: ubuntu-latest

steps:
  - checkout: self
    persistCredentials: true

  - task: GoTool@0
    inputs: { version: '1.22' }

  - script: go build -o ai-code-review .
    displayName: Build reviewer

  - script: ./ai-code-review
    displayName: AI Code Review
    env:
      SYSTEM_ACCESSTOKEN: $(System.AccessToken)
      OPENAI_API_KEY: $(OPENAI_API_KEY)
```

### Option B — packaged task / extension

Package the tool as a reusable Azure DevOps task so any pipeline can call
`AICodeReview@0`. See [sample-azure-pipelines.yml](sample-azure-pipelines.yml)
for a full pipeline using it.

```bash
make build                      # build binaries into task/bin/
npm install -g tfx-cli
tfx extension create --manifest-globs extension/vss-extension.json
```

Upload the resulting `.vsix` to the Visual Studio Marketplace and install it to
your organization. The task binaries are bundled into the `.vsix`, so always run
`make build` before packaging (`task/bin/` is gitignored).

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | yes (unless `MOCK_AI=1`) | OpenAI API key |
| `OPENAI_BASE_URL` | no | Override the API endpoint (e.g. Azure OpenAI) |
| `MOCK_AI` | no | `1` returns a fake finding instead of calling OpenAI |
| `DRY_RUN` | no | `1` prints comments instead of posting them |
| `FAIL_ON` | no | Override `failOn` from config (`blocker`/`major`/`minor`/`info`/`none`) |
| `CONFIG_PATH` | no | Path to the config file (default `.codereview.yml`) |

For local runs, copy [.env.example](.env.example) to `.env`; the local-test
scripts load it for you. The binary itself reads only real environment
variables and never a `.env` file, so configuration in CI comes entirely from
the pipeline. The `SYSTEM_*` and `BUILD_*` variables are provided automatically
by Azure DevOps.

## Building

```bash
make build          # all platforms into task/bin/
make build-linux
make build-windows
make build-darwin
go test ./...
```

## License

[MIT](LICENSE)
