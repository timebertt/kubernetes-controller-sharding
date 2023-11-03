# Getting Started With Controller Sharding

This guide walks you through setting up sharding for Kubernetes controllers and deploying the [webhosting-operator](../webhosting-operator/README.md) as an example controller that supports sharding.

The first steps deploy the sharding components along with a few system components for monitoring, profiling, and so on.
However, without a controller, the sharding components don't do anything.
To experience how they work, the webhosting-operator is deployed in the following steps.
While webhosting-operator is developed in the same repository, it only serves as an example.
Sharding support can be implemented in any other controller and programming language as well, so that it works well with the sharding components from this project.

> [!NOTE]
> The external sharding components are work in progress.
> While the following steps also deploy the new sharding components, they are not activated for now (no `ClusterRing` object is created).
> Instead, the existing sharding implementation in controller-runtime embedded in webhosting-operator is used.

## Quick Start

```bash
# create a local cluster
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
# deploy sharding, monitoring, and system components
make up
# deploy the webhosting-operator
make -C webhosting-operator up
# create some sample websites
k apply -f webhosting-operator/config/samples
# access the grafana dashboards
k -n monitoring port-forward svc/grafana 3000
```

Now, visit the sample websites: http://localhost:8088/project-foo/homepage and http://localhost:8088/project-foo/official.
Also, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1).

## 1. Create a Kubernetes Cluster

### kind (local)

Create a local cluster in docker containers via [kind](https://kind.sigs.k8s.io/) using a provided make target.
It already takes care of configuring the needed port mappings.

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
```

### Shoot Cluster (remote)

Alternatively, you can also create a cluster in the cloud.
If you have a Gardener installation available, you can create a `Shoot` cluster similar to the one in the [sample manifest](../hack/config/shoot.yaml):

```bash
k apply -f hack/config/shoot.yaml
# gardenctl target ...

# deploy external-dns for managing DNS records for monitoring and our webhosting service
k apply --server-side -k config/external-dns
k -n external-dns create secret generic google-clouddns-timebertt-dev --from-literal project=$PROJECT_NAME --from-file service-account.json=$SERVICE_ACCOUNT_FILE
```

## 2. Deploy the Sharding Components

Now it's time to deploy the sharding components along with a few system components for monitoring, profiling, and so on.

Build a fresh image and deploy it using [skaffold](https://skaffold.dev/):

```bash
# one-time build and deploy including port forwarding and log tailing
make up

# or: dev loop (rebuild on trigger after code changes)
make dev
```

Alternatively, deploy pre-built images:

```bash
make deploy TAG=latest
```

## 3. Deploy the webhosting-operator

To experience how controller sharding works, deploy the webhosting-operator as an example controller.

Build a fresh image and deploy it using [skaffold](https://skaffold.dev/):

```bash
# one-time build and deploy including port forwarding and log tailing
make -C webhosting-operator up

# or: dev loop (rebuild on trigger after code changes)
make -C webhosting-operator dev
```

Alternatively, deploy pre-built images:

```bash
make -C webhosting-operator deploy TAG=latest
```

## 4. Create Sample Objects

Create a sample project namespace as well as two websites using two different themes:

```bash
k apply -f webhosting-operator/config/samples
```

Checkout the created websites in the project namespace:

```bash
$ k -n project-foo get website,deploy,svc,ing
NAME                                        THEME      PHASE   AGE
website.webhosting.timebertt.dev/homepage   exciting   Ready   10s
website.webhosting.timebertt.dev/official   lame       Ready   10s

NAME                              READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/homepage-72833b   1/1     1            1           10s
deployment.apps/official-698696   1/1     1            1           10s

NAME                      TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
service/homepage-72833b   ClusterIP   10.43.119.71   <none>        8080/TCP   10s
service/official-698696   ClusterIP   10.43.22.107   <none>        8080/TCP   10s

NAME                                        CLASS   HOSTS   ADDRESS      PORTS   AGE
ingress.networking.k8s.io/homepage-72833b   nginx   *       172.19.0.2   80      10s
ingress.networking.k8s.io/official-698696   nginx   *       172.19.0.2   80      10s
```

Navigate to http://localhost:8088/project-foo/homepage and http://localhost:8088/project-foo/official in your browser to visit the websites.

Optionally, generate some more websites using the [samples-generator](../webhosting-operator/cmd/samples-generator):

```bash
$ k create ns project-bar && k create ns project-baz
# create a random number of websites per project namespace (up to 50 each)
$ cd webhosting-operator
$ go run ./cmd/samples-generator
created 32 Websites in project "project-foo"
created 25 Websites in project "project-bar"
created 23 Websites in project "project-baz"
```

## 5. Access Monitoring Components

You've already deployed a customized installation of [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) including `webhosting-exporter` for observing the operator and its objects created in the previous steps.
To access grafana, get the password and forward the port:

```bash
# get the generated grafana admin password
cat hack/config/monitoring/default/grafana_admin_password.secret.txt

k -n monitoring port-forward svc/grafana 3000
```

Now, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1) at http://127.0.0.1:3000.
Also, explore the controller-runtime and related dashboards!

## 6. Run Load Test Experiments

The [experiment](./cmd/experiment) tool allows executing different load test scenarios for evaluation purposes.

```text
$ cd webhosting-operator
$ go run ./cmd/experiment -h
Usage:
  experiment [command]

Available Scenarios
  basic       Basic load test scenario (15m) that creates roughly 8k websites over 10m
  reconcile   High frequency reconciliation load test scenario (15m) with a static number of websites (10k)
...
```

Run a load test scenario using one of these commands:

```bash
# run the basic scenario from your development machine
go run ./cmd/experiment basic

# build the experiment image and run the basic scenario as a Job on the cluster
make up SKAFFOLD_MODULE=experiment EXPERIMENT_SCENARIO=basic

# use a pre-built experiment image to run the basic scenario as a Job on the cluster
make deploy SKAFFOLD_MODULE=experiment EXPERIMENT_SCENARIO=basic TAG=latest
```

When running load test experiments on the cluster, a `ServiceMonitor` is created to instruct prometheus to scrape `experiment`.
As the tool is based on controller-runtime as well, the controller-runtime dashboards can be used for visualizing the load test scenario and verifying that the tool is able to generate the desired load.
