# Ensure that if GOPATH is set, the GOPATH/{bin,pkg} directory exists. This might not be the case in CI.
# As we will create a symlink against the bin folder we need to make sure that the bin directory is
# present in the GOPATH.
if [ -n "${GOPATH:-}" ] && [ ! -d "$GOPATH/bin" ]; then mkdir -p "$GOPATH/bin"; fi
if [ -n "${GOPATH:-}" ] && [ ! -d "$GOPATH/pkg" ]; then mkdir -p "$GOPATH/pkg"; fi

VIRTUAL_GOPATH="$(mktemp -d)"
trap 'rm -rf "$VIRTUAL_GOPATH"' EXIT

# Setup virtual GOPATH
go mod download && vgopath -o "$VIRTUAL_GOPATH"

export GOPATH="$VIRTUAL_GOPATH"
