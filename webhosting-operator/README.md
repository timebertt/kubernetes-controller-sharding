# webhosting-operator

webhosting-operator is a simple operator developed using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).
It is built for demonstrating and evaluating the implemented sharding design for Kubernetes controllers.

## Sample Operator Requirements

To demonstrate and evaluate the proposed sharding design, an operator is needed that fulfills the following requirements:

- it should be composed of a single controller for one (custom) resource
- in addition to watching its own resources, it needs to watch other relevant objects (e.g. owned objects) as well
  - sharding is more difficult here, so add it as a challenge
- it needs to deal with cluster-scoped objects (that are relevant for multiple namespaced objects)
  - this adds side effects (duplicated cache) which need to be taken care of

## Idea / Introduction

The idea behind this operator is simple: we want to build a web hosting platform on top of Kubernetes.
This means we want to be able to configure websites for our customers in a declarative manner.
The desired state is configured via Kubernetes (custom) resources and the operator takes care to spin up websites and expose them.

There are three resources involved:

- `Namespace`
  - each customer project gets its own namespace
- `Theme` (`webhosting.timebertt.dev`, cluster-scoped)
  - represents an offered theme for customer websites (managed by service admin)
  - configures a font family and color for websites
- `Website` (`webhosting.timebertt.dev`, namespaced)
  - represents a single website a customer orders (managed by the customer in a project namespace)
  - website simply displays the website's name (static)
  - each website references exactly one theme
  - deploys and configures a simple `nginx` deployment
  - exposes the website via service and ingress

## Setup

### TL;DR

All necessary steps for a quick start:

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
make up
k apply -f config/samples
```

Alternatively, use pre-built images:

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
make deploy TAG=latest
k apply -f config/samples
```

Now, visit the sample websites: http://localhost:8088/project-foo/homepage and http://localhost:8088/project-foo/official.
Also, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1).

### 1. Create a Kubernetes Cluster

#### kind (local)

Create a local cluster in docker containers via [kind](https://kind.sigs.k8s.io/) using a provided make target.
It already takes care of deploying the prerequisites and configuring the needed port mappings.

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
```

#### Shoot Cluster (remote)

Alternatively, you can also create a cluster in the cloud. If you have a Gardener installation available, you can create a `Shoot` cluster similar to the one in the [sample manifest](../hack/config/shoot.yaml) and deploy the prerequisites manually:

```bash
k apply -f shoot.yaml
# gardenctl target ...

# deploy external-dns for managing a DNS record for our webhosting service
k apply --server-side -k config/external-dns
k -n external-dns create secret generic google-clouddns-timebertt-dev --from-literal project=$PROJECT_NAME --from-file service-account.json=$SERVICE_ACCOUNT_FILE

# deploy cert-manager for managing TLS certificates
k apply --server-side -k config/cert-manager

# deploy ingress-nginx with service annotations for exposing websites via public dns and requesting a TLS certificate
make deploy-ingress-nginx OVERLAY=shoot
```

### 2. Deploy the Operator

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

### 3. Create Sample Objects

Create a sample project namespace as well as two websites using two different themes:

```bash
k apply -f config/samples
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

Generate some more samples with:
```bash
$ k create ns project-bar && k create ns project-baz
# create a random number of websites per project namespace (up to 50 each)
$ go run ./cmd/samples-generator
created 32 Websites in project "project-foo"
created 25 Websites in project "project-bar"
created 23 Websites in project "project-baz"
```

### 4. Access Monitoring Components

You've already deployed a customized installation of [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) including `webhosting-exporter` for observing the operator and its objects created in the previous steps.

```bash
# get the grafana admin password
cat config/monitoring/default/grafana_admin_password.secret.txt
```

Now, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1) at http://127.0.0.1:3000.
Also, explore the controller-runtime and related dashboards!

### 5. Run Load Test Experiments

The [experiment](./cmd/experiment) tool allows executing different load test scenarios for evaluation purposes.

```text
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
