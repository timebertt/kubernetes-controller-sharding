# Evaluating the Sharding Mechanism

This guide describes how the sharding mechanism implemented in this repository is evaluated and outlines the key results of the evaluation performed in the associated [Master's thesis](https://github.com/timebertt/masters-thesis-controller-sharding).

## Components

The evaluation setup builds upon the [Development and Testing Setup](development.md) but adds a few more components.
To demonstrate and evaluate the implemented sharding mechanisms using a fully functioning controller, a dedicated example operator was developed: the [webhosting-operator](../webhosting-operator/README.md).
While the webhosting-operator is developed in the same repository, it only serves as an example.

After deploying the sharding components using `make deploy` or `make up`, the webhosting-operator can be deployed in a similar manner along with the other evaluation components.
Assuming you're in the repository's root directory:

```bash
# deploy the webhosting-operator using pre-built images
make -C webhosting-operator deploy TAG=latest
# alternatively, build and deploy fresh images
make -C webhosting-operator up
```

To perform a quick test of the webhosting-operator, create some example `Website` objects:

```bash
$ kubectl apply -k webhosting-operator/config/samples
...

$ kubectl -n project-foo get website,deploy,svc,ing -L shard.alpha.sharding.timebertt.dev/clusterring-ef3d63cd-webhosting-operator
NAME                                        THEME      PHASE   AGE   CLUSTERRING-EF3D63CD-WEBHOSTING-OPERATOR
website.webhosting.timebertt.dev/homepage   exciting   Ready   58s   webhosting-operator-5d8d548cb9-qmwc7
website.webhosting.timebertt.dev/official   lame       Ready   58s   webhosting-operator-5d8d548cb9-qq549

NAME                              READY   UP-TO-DATE   AVAILABLE   AGE   CLUSTERRING-EF3D63CD-WEBHOSTING-OPERATOR
deployment.apps/homepage-c1160b   1/1     1            1           57s   webhosting-operator-5d8d548cb9-qmwc7
deployment.apps/official-97b754   1/1     1            1           57s   webhosting-operator-5d8d548cb9-qq549

NAME                      TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE   CLUSTERRING-EF3D63CD-WEBHOSTING-OPERATOR
service/homepage-c1160b   ClusterIP   10.96.83.180    <none>        8080/TCP   58s   webhosting-operator-5d8d548cb9-qmwc7
service/official-97b754   ClusterIP   10.96.193.214   <none>        8080/TCP   58s   webhosting-operator-5d8d548cb9-qq549

NAME                                        CLASS   HOSTS   ADDRESS   PORTS   AGE   CLUSTERRING-EF3D63CD-WEBHOSTING-OPERATOR
ingress.networking.k8s.io/homepage-c1160b   nginx   *                 80      58s   webhosting-operator-5d8d548cb9-qmwc7
ingress.networking.k8s.io/official-97b754   nginx   *                 80      58s   webhosting-operator-5d8d548cb9-qq549
```

You can now visit the created websites at http://localhost:8088/project-foo/homepage and http://localhost:8088/project-foo/official.
You can also visit your [local webhosting dashboard](http://127.0.0.1:3000/d/NbmNpqEnk/webhosting?orgId=1) after forwarding the Grafana port:

```bash
kubectl -n monitoring port-forward svc/grafana 3000
```

This dashboard uses metrics exported by [webhosting-operator](../webhosting-operator/pkg/metrics) about its API objects, i.e., `kube_website_*` and `kube_theme_*`.
There is also a dashboard about the [sharding of websites](http://127.0.0.1:3000/d/7liIybkVk/sharding?orgId=1).

In addition to creating the preconfigured websites, you can also generate some more random websites using the [samples-generator](../webhosting-operator/cmd/samples-generator):

```bash
# create a random number of websites per project namespace (up to 50 each)
$ cd webhosting-operator
$ go run ./cmd/samples-generator
created 32 Websites in project "project-foo"
```

## Load Tests

The [experiment](./cmd/experiment) tool allows executing different scenarios for load testing the webhosting-operator, which are used for evaluating the sharding mechanism:

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

A load test scenario can be executed using one of these commands:

```bash
# run the basic scenario from your development machine (not recommended)
go run ./cmd/experiment basic

# build the experiment image and run the basic scenario as a Job on the cluster
make -C webhosting-operator up SKAFFOLD_MODULE=experiment EXPERIMENT_SCENARIO=basic

# use a pre-built experiment image to run the basic scenario as a Job on the cluster
make -C webhosting-operator deploy SKAFFOLD_MODULE=experiment EXPERIMENT_SCENARIO=basic TAG=latest
```

All scenarios put load on webhosting-operator by creating and manipulating a large amount of `Website` objects.
However, create soo many `Websites` would waste immense compute power just to run thousands of dummy websites.
Hence, webhosting-operator creates `Deployments` of `Websites` in load tests with `spec.replicas=0`.
It also doesn't expose `Websites` created in load tests via `Ingress` objects by setting `spec.ingressClassName=fake`.
Otherwise, this would overload the ingress controller, which is not what the experiment is actually supposed to load test.

When running load test experiments on the cluster, a `ServiceMonitor` is created to instruct prometheus to scrape `experiment`.
As the tool is based on controller-runtime as well, the controller-runtime dashboards can be used for visualizing the load test scenario and verifying that the tool is able to generate the desired load.

After executing a load test experiment, the [measure](../webhosting-operator/cmd/measure) tool can be used for retrieving the key metrics from Prometheus.
It takes a configurable set of measurements in the form of Prometheus queries and stores them in CSV-formatted files for further analysis (with `numpy`) and visualization (with `matplotlib`).
Please see the [results directory](https://github.com/timebertt/masters-thesis-controller-sharding/tree/main/results) in the Master's thesis' repository for the exact measurements taken.

## Experiment Setup

As a local kind cluster cannot handle such high load, a remote cluster is used to perform the load test experiments.
For this, a [Gardener](https://github.com/gardener/gardener) installation on [STACKIT](https://www.stackit.de/en/) is used to create a cluster based on the [sample manifest](../hack/config/shoot.yaml).
[external-dns](https://github.com/kubernetes-sigs/external-dns) is used for publicly exposing the monitoring and continuous profiling endpoints, as well as `Websites` created outside of load test experiments.

```bash
# gardenctl target --garden ...
kubectl apply -f hack/config/shoot.yaml

# gardenctl target --shoot ...
kubectl apply --server-side -k hack/config/external-dns
kubectl -n external-dns create secret generic google-clouddns-timebertt-dev --from-literal project=$PROJECT_NAME --from-file service-account.json=$SERVICE_ACCOUNT_FILE

# gardenctl target --control-plane
kubectl apply --server-side -k hack/config/policy/controlplane
```

In addition to the described components, [kyverno](https://github.com/kyverno/kyverno) is deployed to the cluster itself (shoot cluster) and to the control plane (seed cluster).
In the cluster itself, kyverno policies are used for scheduling the sharder and webhosting-operator to the dedicated `sharding` worker pool and experiment to the dedicated `experiment` worker pool.
This makes sure that these components run on machines isolated from other system components and don't content for compute resources during load tests.
Furthermore, kyverno policies are added to the control plane to ensure a static size of etcd and kube-apiserver (static requests/limits to disable vertical autoscaling, 4 replicas of kube-apiserver to disable horizontal autoscaling) and schedule them to a dedicated worker pool with more CPU cores per machine.
This is done to make load test experiments more stable and and their results more reproducible.

## Key Results

> [!NOTE]
> These are preliminary results from a first set of test runs.  
> TODO: update these once the full evaluation is completed.

The following graphs compare CPU and memory usage of the components in three different setups when running the `basic` experiment scenario (~8,000 websites created over 10m):

- external sharder: 3 webhosting-operator pods (shards) + 2 sharder pods (the new approach implemented in this repository, second iteration for the Master's thesis)
- internal sharder: 3 webhosting-operator pods (3 shards, 1 acts as the sharder) (the old approach implemented, first iteration for the study project)
- singleton: 1 webhosting-operator pod (traditional leader election setup without sharding)

![CPU comparison](assets/comparison-cpu.jpg)

![Memory comparison](assets/comparison-memory.jpg)

The new external sharding approach proves to scale best.
The individual shards consume about a third of the singleton controller's usage (close to optimum).
Also, the sharder pods consume a low static amount of resources. 
Most importantly, the sharder's resource usage is independent of the number of sharded objects.
