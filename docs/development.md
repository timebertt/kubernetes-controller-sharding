# Development and Testing Setup

This document explains more details of the development and testing setup that is also presented in [Getting Started With Controller Sharding](getting-started.md).

## Development Cluster

The setup's basis is a local [kind](https://kind.sigs.k8s.io/) cluster.
This simplifies developing and testing the project as it comes without additional cost, can be thrown away easily, and one doesn't need to push development images to a remote registry.
In other words, there are no prerequisites for getting started with this project other than a [Go](https://go.dev/) and [Docker](https://www.docker.com/) installation.

```bash
# create a local cluster
make kind-up
# target the kind cluster
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml

# delete the local cluster
make kind-down
```

If you want to use another cluster for development (e.g., a remote cluster) simply set the `KUBECONFIG` environment variable as usual and all make commands will target the cluster pointed to by your kubeconfig.
Note that you might need to push images to a remote registry though.

## Components

The development setup reuses the deployment manifests of the main sharding components developed in this repository, located in [`config`](../config).
See [Install the Sharding Components](installation.md).

It also includes the [checksum-controller](../cmd/checksum-controller) as an example sharded controller (see [Implement Sharding in Your Controller](implement-sharding.md)) and the [webhosting-operator](../webhosting-operator/README.md) (see [Evaluating the Sharding Mechanism](evaluation.md)).

Apart from this, the development setup also includes some external components, located in [`hack/config`](../hack/config).
This includes [cert-manager](https://cert-manager.io/), [ingress-nginx](https://kubernetes.github.io/ingress-nginx/), [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus), [kyverno](https://kyverno.io/), and [parca](https://parca.dev/).
These components are installed for a seamless development and testing experience but also for this project's [Evaluation](evaluation.md) on a remote cluster in the cloud.

## Deploying, Building, Running Using Skaffold

Use `make deploy` to deploy all components with pre-built images using [skaffold](https://skaffold.dev/).
You can overwrite the used images via make variables, e.g., the `TAG` variable:

```bash
make deploy TAG=latest
```

For development, skaffold can build fresh images based on your local changes using [ko](https://ko.build/), load them into your local cluster, and deploy the configuration:

```bash
make up
```

Alternatively, you can also start a skaffold-based dev loop which can automatically rebuild and redeploy images as soon as source files change:

```bash
make dev
# runs initial build and deploy...
# press any key to trigger a fresh build after changing sources
```

If you're not working with a local kind cluster, you need to set `SKAFFOLD_DEFAULT_REPO` to a registry that you can push the dev images to:

```bash
make up SKAFFOLD_DEFAULT_REPO=ghcr.io/timebertt/dev-images
```

Remove all components from the cluster:

```bash
make down
```

For any skaffold-based make command, you can set `SKAFFOLD_MODULE` to target only a specific part of the [skaffold configuration](../hack/config/skaffold.yaml):

```bash
make dev SKAFFOLD_MODULE=sharder
```

## Running on the Host Machine

Instead of running the sharder in the cluster, you can also run it on your host machine targeting your local kind cluster.
This doesn't deploy all components as before but only cert-manager for injecting the webhook's CA bundle.
Assuming a fresh kind cluster:

```bash
make run
```

Now, create the `ControllerRing` and run a local `checksum-controller`:

```bash
make run-checksum-controller
```

You should see that the shard successfully announced itself to the sharder:

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/controllerring,alpha.sharding.timebertt.dev/state
NAME                           HOLDER                         AGE   CONTROLLERRING        STATE
checksum-controller-lhrlt6h4   checksum-controller-lhrlt6h4   6s    checksum-controller   ready

$ kubectl get controllerring
NAME                  READY   AVAILABLE   SHARDS   AGE
checksum-controller   True    1           1        13s
```

Running the `checksum-controller` locally gives you the option to test non-graceful termination, i.e., a scenario where the shard fails to renew its lease in time.
Simply press `Ctrl-C` twice:

```bash
make run-checksum-controller
...
^C2023-11-24T15:16:50.948+0100	INFO	Shutting down gracefully in 2 seconds, send another SIGINT or SIGTERM to shutdown non-gracefully
^Cexit status 1
```

## Testing the Sharding Setup

Independent of the used setup (skaffold-based or running on the host machine), you should be able to create sharded `Secrets` in the `default` namespace as configured in the `example` `ControllerRing`.
The `ConfigMaps` created by the `checksum-controller` should be assigned to the same shard as the owning `Secret`:

```bash
$ kubectl create secret generic foo --from-literal foo=bar
secret/foo created

$ kubectl get cm,secret -L shard.alpha.sharding.timebertt.dev/checksum-controller
NAME                      DATA   AGE   CHECKSUM-CONTROLLER
configmap/checksums-foo   1      1s    checksum-controller-lhrlt6h4

NAME         TYPE     DATA   AGE   CHECKSUM-CONTROLLER
secret/foo   Opaque   1      1s    checksum-controller-lhrlt6h4
```

## Monitoring

When using the skaffold-based setup, you also get a full monitoring setup for observing and analyzing the components' resource usage.

To access the monitoring dashboards and metrics in Grafana, simply forward its port and open http://localhost:3000/ in your browser:

```bash
kubectl -n monitoring port-forward svc/grafana 3000 &
```

The password for Grafana's `admin` user is written to `hack/config/monitoring/default/grafana_admin_password.secret.txt`.

Be sure to check out the controller-runtime dashboard: http://localhost:3000/d/PuCBL3zVz/controller-runtime-controllers

## Continuous Profiling

To dig deeper into the components' resource usage, you can deploy the continuous profiling setup based on [Parca](https://parca.dev/):

```bash
make up SKAFFOLD_MODULE=profiling SKAFFOLD_PROFILE=profiling
```

To access the profiling data in Parca, simply forward its port and open http://localhost:7070/ in your browser:

```bash
kubectl -n parca port-forward svc/parca 7070 &
```

For accessing Parca through its `Ingress`, use the basic auth password for the `parca` user from `hack/config/profiling/parca_password.secret.txt`.

Note that the Parca deployment doesn't implement retention for profiling data.
I.e., the Parca data volume will grow infinitely as long as Parca is running.
To shut down Parca after analyzing the collected profiles and destroying the persistent volume use the following command:

```bash
make down SKAFFOLD_MODULE=profiling SKAFFOLD_PROFILE=profiling
```
