# webhosting-operator

webhosting-operator is a simple operator developed using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).
It is built for demonstrating and evaluating the implemented sharding mechanisms for Kubernetes controllers, see [Evaluating the Sharding Mechanism](../docs/evaluation.md).

The webhosting-operator is developed in a dedicated Go module so that dependencies of the actual sharding components can be managed independently of the example operator.
Also, webhosting-operator does not reuse the sharder components implemented in the main module as described in [Implement Sharding in Your Controller](../docs/implement-sharding.md).
In fact, webhosting-operator doesn't have a Go dependency on the main module at all.

The reason for this is, that webhosting-operator was already used for evaluation of the project's first iteration that ran an internal sharder ("sidecar controller") in one of the shards (via leader election), implemented in my [controller-runtime fork](https://github.com/timebertt/controller-runtime/tree/sharding-0.15).
The "legacy" controller-runtime implementation is now compatible with the external sharder from this repository however, i.e., you can run webhosting-operator shards for a `ClusterRing` by setting the `SHARD_MODE=shard` and `LEADER_ELECT=false` env vars.

In the future, webhosting-operator could be adapted to follow the [Implement Sharding in Your Controller](../docs/implement-sharding.md) guide and reuse sharder components of the main module.
For now, the webhosting-operator is kept in the same repository for simple evaluation of the project.
However, it might be extracted into a dedicated repository later on if necessary.

## Sample Operator Requirements

To demonstrate and evaluate the proposed sharding design, an operator is needed that fulfills the following requirements:

- it should be composed of a single controller for one (custom) resource
- in addition to watching its own resources, it needs to watch other relevant objects (e.g. owned objects) as well
  - sharding is more difficult here, so add it as a challenge
- it needs to deal with cluster-scoped objects (that are relevant for multiple namespaced objects)
  - this adds side effects (duplicated cache) which need to be taken care of

## Idea / Introduction

The idea behind this operator is simple: we want to build a web hosting platform on top of Kubernetes.
I.e., we want to be able to configure websites for our customers in a declarative manner.
The desired state is configured via Kubernetes (custom) resources and the operator takes care to spin up websites and expose them.

There are three resources involved:

- `Namespace`
  - each customer project gets its own namespace
  - a project namespace is identified by the `webhosting.timebertt.dev/project=true` label
- `Theme` (API group `webhosting.timebertt.dev`, cluster-scoped)
  - represents an offered theme for customer websites (managed by service admin)
  - configures a font family and color for websites
- `Website` (API group `webhosting.timebertt.dev`, namespaced)
  - represents a single website a customer orders (managed by the customer in a project namespace)
  - website simply displays the website's name (static)
  - each website references exactly one theme
  - deploys and configures a simple `nginx` deployment
  - exposes the website via service and ingress

## Setup

To test controller sharding with the webhosting-operator as an example controller, see [Evaluation](../docs/evaluation.md).
