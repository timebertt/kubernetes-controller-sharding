#!/usr/bin/env bash

if ! command -v cfssl &>/dev/null ; then
  echo "cfssl not found, install it from https://github.com/cloudflare/cfssl"
  exit 1
fi

cd "$(dirname "$0")"

rm -f *.pem

cfssl gencert -config config.json -initca webhook-ca.json | cfssljson -bare webhook-ca

cfssl gencert -config config.json -ca=webhook-ca.pem -ca-key=webhook-ca-key.pem -profile=server webhook-server.json | cfssljson -bare webhook-server

rm *.csr
