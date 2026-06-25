# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nitpicker .

# ---- Runtime stage ----
# git is required to compute the diff; ca-certificates for HTTPS to OpenAI/ADO.
FROM alpine:3.22

RUN apk add --no-cache git ca-certificates

# The mounted checkout is owned by the host user, which trips git's dubious
# ownership guard. Mark all paths safe so the diff commands run.
RUN git config --system --add safe.directory '*'

# The reviewed repo is mounted here at runtime. The binary defaults the repo path
# to BUILD_REPOSITORY_LOCALPATH or ".", so `docker run -v "$PWD:/repo"` just works.
WORKDIR /repo

COPY --from=build /out/nitpicker /usr/local/bin/nitpicker

ENTRYPOINT ["/usr/local/bin/nitpicker"]
