# syntax=docker/dockerfile:1
# The Go binaries are architecture independent to build, so the build stage
# runs on the builder's own platform and Go cross-compiles for TARGETARCH.
# Only the final stage runs as the target (emulated for arm64 on an amd64
# builder), which keeps multi-platform releases fast.
FROM --platform=$BUILDPLATFORM golang:1.26.11-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
# Empty lets build-binaries.sh size parallelism from the builder's CPUs and memory.
ARG LOOPFORGE_BUILD_JOBS=
ENV LOOPFORGE_BUILD_JOBS=${LOOPFORGE_BUILD_JOBS}
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
# Ref updates must not invalidate the Go build cache.
COPY --exclude=.git . .
# The runner and gate binaries are built for every OS/arch regardless; GOOS and
# GOARCH pick the architecture of the binaries that run inside this image.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build GOOS=${TARGETOS} GOARCH=${TARGETARCH} bash scripts/build-binaries.sh --helpers /out
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build GOOS=${TARGETOS} GOARCH=${TARGETARCH} bash scripts/build-binaries.sh --platform /out
# The personal build runs no agent: it serves the console, the requirement
# list, the machine inventory and the usage console. The image therefore holds
# the platform binaries and the tools they shell out to — git for the
# repository operations, curl and python3 for the health check.
FROM debian:bookworm-slim
RUN --mount=type=cache,target=/var/cache/apt apt-get update && apt-get install -y --no-install-recommends ca-certificates git openssh-client bash curl python3 \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# The SQLite database file lives on the platform-data volume at /data; the
# directory exists in the image so the platform can create the file on a
# first start without a database server.
RUN mkdir -p /data
COPY --from=build /out/ /app/bin/
COPY LICENSE THIRD_PARTY_NOTICES.md /app/
COPY third_party/go/ /app/third_party/go/
ENV PATH="/app/bin:${PATH}"
# An explicit LOOPFORGE_COMMIT wins. Otherwise read HEAD from the build context
# so `docker build` stamps the same corner label as the release workflow.
ARG LOOPFORGE_COMMIT=
ARG LOOPFORGE_RELEASE_VERSION=
ENV LOOPFORGE_RELEASE_VERSION=${LOOPFORGE_RELEASE_VERSION}
RUN --mount=type=bind,source=.,target=/ctx,ro \
    sh /ctx/scripts/image-commit.sh /ctx /app/loopforge-commit
EXPOSE 8080
ENTRYPOINT ["/app/bin/agentflow"]
