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

### 1. Create a Kubernetes Cluster

Create a local cluster in docker containers via [k3d](https://k3d.io/) using a provided make target.
It already takes care of deploying the prerequisites and configuring the needed port mappings.

```bash
make k3d-up
```

Alternatively, you can also create a cluster in the cloud. If you have a Gardener installation available, you can create a `Shoot` cluster similar to the one in the [sample manifest](./shoot.yaml):

```bash
k apply -f shoot.yaml
```

In this case, you need to manually deploy the prerequisites.
An ingress controller like [ingress-nginx](https://github.com/kubernetes/ingress-nginx/) is needed to expose `Websites`. Deploy it via:

```bash
k apply --server-side -k config/ingress-nginx/with-dns # including service annotations for public dns
```

### 2. Deploy the Operator

Deploy `webhosting-operator` using the `latest` tag:

```bash
make deploy

# or: configure the operator to make ingresses available via public dns 
make deploy WITH_DNS=true
```

Alternatively, build the image and deploy it using [skaffold](https://skaffold.dev/):

```bash
# one-time deploy
skaffold run

# or: dev-loop (rebuild on code changes)
skaffold dev
```

### 3. Create Sample Objects

Create sample project namespace as well as two websites using two different themes:

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

Navigate to [localhost:8088/project-foo/homepage](http://localhost:8088/project-foo/homepage) and [localhost:8088/project-foo/official](http://localhost:8088/project-foo/official) in your browser to visit the websites.

## 4. Deploy Monitoring Components

Deploy a customized installation of [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) for observing the operator:

```bash
k apply --server-side -k config/monitoring/crds
k wait crd -l app.kubernetes.io/name=prometheus-operator --for=condition=NamesAccepted --for=condition=Established
k apply --server-side -k config/monitoring
```

Access grafana and prometheus:
```bash
k port-forward -n monitoring svc/grafana 3000
k port-forward -n monitoring svc/prometheus-k8s 9090
```
