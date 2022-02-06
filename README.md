# kubernetes-controller-sharding

Experiment for sharding in Kubernetes controllers

## TL;DR

Try to distribute reconciliation of Kubernetes objects across multiple controller instances.
Remove the limitation to have only one active replica (leader) per controller.

## Motivation

TBA

## Test setup

TBA

[webhosting-operator](webhosting-operator) is developed as a demo use case and for evaluating different sharding approaches.
