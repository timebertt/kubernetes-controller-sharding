#!/usr/bin/env bash

dir="$(dirname "$0")"
password_file="$dir/parca_password.secret.txt"
auth_file="$dir/parca_auth.secret.txt"

[ -f "$password_file" ] && [ -f "$auth_file" ] && exit 0
cat /dev/urandom | tr -dc "a-zA-Z0-9" | head -c 32 > "$password_file"
cat "$password_file" | htpasswd -i -c "$auth_file" parca
