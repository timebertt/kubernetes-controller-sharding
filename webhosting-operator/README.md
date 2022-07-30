# webhosting-operator

webhosting-operator is a simple operator developed using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).
It is built for demonstrating different sharding approaches for Kubernetes controllers.

## Test Setup Requirements

In order to demonstrate and compare different sharding approaches, an operator is needed that fulfills the following requirements:

- it needs to act on multiple custom resources
  - for demonstrating the sharding by resource type approach
- in addition to watching its own resources, it needs to watch other objects (e.g. owned objects) as well
  - sharding will be difficult here, so add it as a challenge
- it needs to deal with cluster-scoped objects (that are relevant for multiple namespaced objects)
  - this adds side effects (duplicated cache) which need to be taken care of

## Idea / Introduction

The idea behind this operator is simple: we want to build a web-hosting platform on top of Kubernetes.
This means, we want to be able to configure websites for our customers in a declarative manner.
The desired state is configured via Kubernetes (custom) resources and the operator takes care to spin up websites and expose them.

There are three resources involved:

- `Namespace`
  - each customer project gets its own namespace
- `Theme` (`webhosting.timebertt.dev`, cluster-scoped)
  - represents an offered theme for customer websites (managed by service admin)
  - configures a font family and color for websites
- `Website` (`webhosting.timebertt.dev`, namespaced)
  - represents a single website a customer orders (managed by customer in a project namespace)
  - website simply displays the website's name (static)
  - each website references exactly one theme
  - deploys and configures a simple `nginx` deployment
  - exposes the website via service and ingress

## Setup

### TL;DR

All necessary steps for a quick start:

```bash
make k3d-up
make up
# in a different terminal
export KUBECONFIG=$PWD/dev/k3d_kubeconfig.yaml
k apply -f config/samples
```

Alternatively, use pre-built images (`latest`):

```bash
make k3d-up
export KUBECONFIG=$PWD/dev/k3d_kubeconfig.yaml
make deploy
k apply -f config/samples
make deploy-monitoring
k port-forward -n monitoring svc/grafana 3000
```

Now, visit the sample websites: http://localhost:8088/project-foo/homepage and http://localhost:8088/project-foo/official.
Also, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1&refresh=10s).

### 1. Create a Kubernetes Cluster

#### k3d (local)

Create a local cluster in docker containers via [k3d](https://k3d.io/) using a provided make target.
It already takes care of deploying the prerequisites and configuring the needed port mappings.

```bash
make k3d-up
export KUBECONFIG=$PWD/dev/k3d_kubeconfig.yaml
```

#### Shoot Cluster (remote)

Alternatively, you can also create a cluster in the cloud. If you have a Gardener installation available, you can create a `Shoot` cluster similar to the one in the [sample manifest](./shoot.yaml) and deploy the prerequisites manually:

```bash
k apply -f shoot.yaml
# gardenctl target ...
# deploy ingress-nginx with service annotations for exposing websites via public dns
make deploy-ingress-nginx OVERLAY=with-dns
make deploy-kyverno
```

### 2. Deploy the Operator

Deploy `webhosting-operator` using the `latest` tag:

```bash
make deploy

# or: configure the operator to make ingresses available via public dns 
make deploy OVERLAY=with-dns
```

Alternatively, build a fresh image and deploy it using [skaffold](https://skaffold.dev/):

```bash
# one-time deploy
skaffold run -m webhosting-operator

# or: dev loop (rebuild on code changes)
skaffold dev -m webhosting-operator
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

You can also check out the publicly hosted `Websites` at https://webhosting.timebertt.dev/project-foo/homepage and https://webhosting.timebertt.dev/project-foo/official.

Generate some more samples with:
```bash
$ k create ns project-bar project-baz
$ go run ./cmd/samples-generator # create a random amount of websites per namespace (up to 50 each)
created 32 Websites in project "project-foo"
created 25 Websites in project "project-bar"
created 23 Websites in project "project-baz"
```

### 4. Deploy Monitoring Components

Deploy a customized installation of [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) including `webhosting-exporter` for observing the operator and its objects:

```bash
# use the latest tag for webhosting-exporter
make deploy-monitoring

# access grafana and prometheus
k port-forward -n monitoring svc/grafana 3000
k port-forward -n monitoring svc/prometheus-k8s 9090

# get the grafana admin password
cat config/monitoring/grafana_admin_pass.secret.txt
```

Alternatively, build a fresh image and deploy it using [skaffold](https://skaffold.dev/):

```bash
# one-time deploy
skaffold run -m monitoring --port-forward=user

# or: dev loop (rebuild on code changes)
skaffold dev -m monitoring --port-forward=user
```

Now, visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1&refresh=10s) at http://127.0.0.1:3000.

You can also visit the [public Grafana dashboard](https://grafana.webhosting.timebertt.dev/d/NbmNpqEnk/webhosting?orgId=1&refresh=10s) to see what's currently going on in my cluster. 
