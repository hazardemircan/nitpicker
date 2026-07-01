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

If a file cannot be reviewed at all — for example the OpenAI quota is exhausted
(HTTP 429 `insufficient_quota`) or the API key is invalid — the step **fails
loudly** rather than reporting a clean review. An un-reviewed PR is never treated
as a passing one. Set `FAIL_ON_REVIEW_ERROR=0` to downgrade this to a warning.

## Requirements

- Go 1.22+ (to build)
- An OpenAI API key
- An Azure DevOps pipeline with PR triggers

## Configuration

Review behavior is controlled by a `.codereview.yml` file at the root of the
repository being reviewed. All fields are optional; sensible defaults are used
when the file is absent, so the tool works with zero configuration.

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

### Option C — prebuilt Docker image (no clone, no compile)

A container image is published to the GitHub Container Registry, so a pipeline
can pull and run it directly instead of cloning the source and compiling Go on
every PR. This removes the Go toolchain install + `git clone` + `go build`
overhead from each run.

```
ghcr.io/hazardemircan/nitpicker:latest
```

Run it against a checked-out repository by mounting the checkout at `/repo`:

```yaml
pool:
  vmImage: ubuntu-latest

steps:
  - checkout: self
    persistCredentials: true   # lets nitpicker call the Azure DevOps REST API
    fetchDepth: 0              # full history so the diff merge-base resolves

  - script: |
      docker run --rm \
        -e SYSTEM_ACCESSTOKEN \
        -e OPENAI_API_KEY \
        -e SYSTEM_TEAMFOUNDATIONCOLLECTIONURI \
        -e SYSTEM_TEAMPROJECT \
        -e BUILD_REPOSITORY_ID \
        -e SYSTEM_PULLREQUEST_PULLREQUESTID \
        -e SYSTEM_PULLREQUEST_TARGETBRANCHNAME \
        -e BUILD_REPOSITORY_LOCALPATH=/repo \
        -v "$(Build.Repository.LocalPath):/repo" \
        ghcr.io/hazardemircan/nitpicker:latest
    displayName: AI Code Review
    env:
      SYSTEM_ACCESSTOKEN: $(System.AccessToken)
      OPENAI_API_KEY: $(OPENAI_API_KEY)
```

Notes:

- `checkout` still runs on the host agent; only nitpicker runs in the container.
  The host checkout is mounted read/write at `/repo` and
  `BUILD_REPOSITORY_LOCALPATH=/repo` points the tool at it (the host path the
  pipeline sets by default does not exist inside the container).
- The image bundles `git`, so the diff is computed inside the container.
- Images are published by
  [.github/workflows/docker-publish.yml](.github/workflows/docker-publish.yml)
  when a version tag (`v*`) is pushed. `:latest` always points at the newest
  release; merges to `main` do not publish a new image.

Other CI systems (GitHub Actions, GitLab CI, plain `docker run` locally) work
the same way: provide the environment variables and mount the checkout at
`/repo`. To pin a version, replace `:latest` with a tag such as `:v1.0.0` or a
`:sha-<short-sha>` tag.

The image is based on Alpine and contains only `git`, `ca-certificates`, and the
static `nitpicker` binary. Any remaining scanner findings are in the upstream
`git`/`curl` Alpine packages with no fix released yet; a rebuild picks up fixes
as Alpine ships them.

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | yes (unless `MOCK_AI=1`) | OpenAI API key |
| `OPENAI_BASE_URL` | no | Override the API endpoint (e.g. Azure OpenAI) |
| `MOCK_AI` | no | `1` returns a fake finding instead of calling OpenAI |
| `DRY_RUN` | no | `1` prints comments instead of posting them |
| `FAIL_ON` | no | Override `failOn` from config (`blocker`/`major`/`minor`/`info`/`none`) |
| `FAIL_ON_REVIEW_ERROR` | no | `0` downgrades an incomplete review (API/quota errors) to a warning instead of failing the step. Defaults to failing. |
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

### Docker image

```bash
make docker-build                                   # build ghcr.io/hazardemircan/nitpicker:latest
make docker-push IMAGE=ghcr.io/youruser/nitpicker TAG=v1.0.0   # build + push your own

# or plain docker, mounting a checkout to review it locally:
docker build -t nitpicker .
docker run --rm -e OPENAI_API_KEY -e MOCK_AI=1 -e DRY_RUN=1 \
  -e BUILD_REPOSITORY_LOCALPATH=/repo -v "$PWD:/repo" nitpicker
```

In CI the image is built and pushed to GHCR automatically. See
[Option C](#option-c--prebuilt-docker-image-no-clone-no-compile) and
[.github/workflows/docker-publish.yml](.github/workflows/docker-publish.yml).

## License

[MIT](LICENSE)
