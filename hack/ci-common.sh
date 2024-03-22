export_artifacts() {
  [ -n "${ARTIFACTS:-}" ] || return 0

  mkdir -p "$ARTIFACTS"
  cluster_name=sharding
  echo "> Exporting logs of kind cluster '$cluster_name'"
  kind export logs "$ARTIFACTS" --name "$cluster_name" || true

  echo "> Exporting events of kind cluster '$cluster_name'"
  export_events
}

export_events() {
  local dir="$ARTIFACTS/events"
  mkdir -p "$dir"

  while IFS= read -r namespace; do
    kubectl -n "$namespace" get event --sort-by=lastTimestamp >"$dir/$namespace.log" 2>&1 || true
  done < <(kubectl get ns -oname | cut -d/ -f2)
}
