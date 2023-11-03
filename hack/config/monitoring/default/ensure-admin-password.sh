#!/usr/bin/env bash

dir="$(dirname "$0")"
file="$dir/grafana_admin_password.secret.txt"

[ -f "$file" ] && exit 0
cat /dev/urandom | tr -dc "a-zA-Z0-9" | head -c 32 > "$file"
