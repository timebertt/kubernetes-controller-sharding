# Install the Sharding Components

This guide walks you through installing the sharding components from this repository in your cluster.
This procedure is independent of the controller that you want to use sharding for.

## Main Components (required)

For now, only [kustomize](https://kustomize.io/) is supported as the deployment tool.
All deployment manifests for this repository's components are located in [`config`](../config) and can be used from there.

The `config/default` variant holds the default configuration for the sharding components.
It requires [cert-manager](https://cert-manager.io/) to be installed in the cluster for managing the webhook certificates.
Apply it using `kubectl`:

```bash
kubectl apply --server-side -k "https://github.com/timebertt/kubernetes-controller-sharding//config/default?ref=main"
```

You can customize the configuration using the usual kustomize mechanisms:

```bash
cat >kustomization.yaml <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- https://github.com/timebertt/kubernetes-controller-sharding//config/default?ref=main

images:
- name: ghcr.io/timebertt/kubernetes-controller-sharding/sharder
  newTag: latest

replicas:
- name: sharder
  count: 3
EOF

kubectl apply --server-side -k .
```

If you can't apply the default configuration, e.g., because you don't want to use cert-manager for managing the webhook certificates, you can also apply the dedicated configurations individually.
First apply the CRD and `sharding-system` namespace using the `config/crds` configuration, then apply the `sharder` itself using the `config/sharder` configuration.
Be sure to mount your webhook server cert to `/tmp/k8s-webhook-server/serving-certs/tls.{crt,key}`.

## Monitoring (optional)

`config/monitoring` contains a `ServiceMonitor` for configuring metrics scraping for the sharder using the [prometheus-operator](https://prometheus-operator.dev/).
See [Monitoring the Sharding Components](monitoring.md) for more information on the exposed metrics.
