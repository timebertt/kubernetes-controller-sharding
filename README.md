# kubernetes-controller-sharding

Experiment for sharding in Kubernetes controllers

This study project is part of my master's studies in Computer Science at [DHBW CAS](https://cas.dhbw.de).
You can find the thesis belonging to this implementation in the repository [thesis-controller-sharding](https://github.com/timebertt/thesis-controller-sharding).

The controller sharding implementation itself is done in a generic way in [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
It is currently located in the `sharding` branch of my fork: https://github.com/timebertt/controller-runtime/tree/sharding.
This repository contains a sample operator using the sharding implementation for demonstration and evaluation purposes.

## TL;DR

Try to distribute reconciliation of Kubernetes objects across multiple controller instances.
Remove the limitation to have only one active replica (leader) per controller.

## Motivation

Typically, [Kubernetes controllers](https://kubernetes.io/docs/concepts/architecture/controller/) use a leader election mechanism to determine a *single* active controller instance (leader).
When deploying multiple instances of the same controller, there will only be one active instance at any given time, other instances will be in stand-by.
This is done to prevent controllers from performing uncoordinated and conflicting actions (reconciliations).

If the current leader goes down and loses leadership (e.g. network failure, rolling update) another instance takes over leadership and becomes the active instance.
Such setup can be described as an "active-passive HA-setup". It minimizes "controller downtime" and facilitates fast fail-overs.
However, it cannot be considered as "horizontal scaling" as work is not distributed among multiple instances.

This restriction imposes scalability limitations for Kubernetes controllers. 
I.e., the rate of reconciliations, amount of objects, etc. is limited by the machine size that the active controller runs on and the network bandwidth it can use.
In contrast to usual stateless applications, one cannot increase the throughput of the system by adding more instances (scaling horizontally) but only by using bigger instances (scaling vertically).

This project explores approaches for distributing reconciliation of Kubernetes objects across multiple controller instances. It attempts to lift the restriction of having only one active replica per controller.
For this, mechanisms are required for determining which instance is responsible for which object to prevent conflicting actions.
The project evaluates if and how proven sharding mechanisms from the field of distributed databases can be applied to this problem.

## Test setup

[webhosting-operator](webhosting-operator) is built as a demo controller for demonstrating and evaluating different sharding approaches for Kubernetes controllers.
It also includes a setup for monitoring experiments with the operator using popular monitoring tools for Kubernetes as well as a [custom metrics exporter](./webhosting-operator/cmd/webhosting-exporter).
