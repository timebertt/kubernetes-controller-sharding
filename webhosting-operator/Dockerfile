# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace

# `skaffold debug` sets SKAFFOLD_GO_GCFLAGS to disable compiler optimizations
ARG SKAFFOLD_GO_GCFLAGS
ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -gcflags="${SKAFFOLD_GO_GCFLAGS}" -o manager ./cmd/webhosting-operator && \
    go build -gcflags="${SKAFFOLD_GO_GCFLAGS}" -o webhosting-exporter ./cmd/webhosting-exporter

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS base

# Define GOTRACEBACK to mark this container as using the Go language runtime
# for `skaffold debug` (https://skaffold.dev/docs/workflows/debug/).
ENV GOTRACEBACK=single

WORKDIR /
USER 65532:65532

FROM base AS webhosting-operator

COPY --from=builder /workspace/manager .
ENTRYPOINT ["/manager"]

FROM base AS webhosting-exporter

COPY --from=builder /workspace/webhosting-exporter .
ENTRYPOINT ["/webhosting-exporter"]
