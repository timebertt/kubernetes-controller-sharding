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

To test controller sharding with the webhosting-operator as an example controller, follow [this guide](../docs/getting-started.md).
