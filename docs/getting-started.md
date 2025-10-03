# Getting Started With Controller Sharding

This guide walks you through getting started with controller sharding in a local cluster.

It sets up the sharder and an example sharded controller so that you can see the components in action.
This is great for trying out the project for the first time and learning about the basic concepts.

## Setup

Create a local cluster using [kind](https://kind.sigs.k8s.io/) and deploy all components:

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
make deploy TAG=latest
```

The sharder is running in the `sharding-system` namespace and the example shard (checksum-controller) is deployed to the `default` namespace:

```bash
$ kubectl -n sharding-system get po
NAME                      READY   STATUS    RESTARTS   AGE
sharder-99fcf97b4-hpm6w   1/1     Running   0          17s
sharder-99fcf97b4-zr7rj   1/1     Running   0          17s
$ kubectl get po
TODO
NAME                                  READY   STATUS    RESTARTS   AGE
checksum-controller-c95c4fdb6-7jb2v   1/1     Running   0          18s
checksum-controller-c95c4fdb6-hv8pb   1/1     Running   0          18s
checksum-controller-c95c4fdb6-rtvrm   1/1     Running   0          18s
```

## The `ControllerRing` and `Lease` Objects

We can see that the `ControllerRing` object is ready and reports 3 available shards out of 3 total shards:

```bash
$ kubectl get controllerring checksum-controller
NAME                  READY   AVAILABLE   SHARDS   AGE
checksum-controller   True    3           3        25s
```

All shards announce themselves to the sharder by maintaining an individual `Lease` object with the `alpha.sharding.timebertt.dev/controllerring` label.
We can observe that the sharder recognizes all shards as available by looking at the `alpha.sharding.timebertt.dev/state` label:

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/controllerring,alpha.sharding.timebertt.dev/state
TODO
NAME                                  HOLDER                                AGE   CONTROLLERRING        STATE
checksum-controller-c95c4fdb6-7jb2v   checksum-controller-c95c4fdb6-7jb2v   44s   checksum-controller   ready
checksum-controller-c95c4fdb6-hv8pb   checksum-controller-c95c4fdb6-hv8pb   44s   checksum-controller   ready
checksum-controller-c95c4fdb6-rtvrm   checksum-controller-c95c4fdb6-rtvrm   44s   checksum-controller   ready
```

The `ControllerRing` object specifies which API resources should be sharded.
Optionally, it allows selecting the namespaces in which API resources are sharded:

```yaml
apiVersion: sharding.timebertt.dev/v1alpha1
kind: ControllerRing
metadata:
  name: checksum-controller
spec:
  resources:
  - group: ""
    resource: secrets
    controlledResources:
    - group: ""
      resource: configmaps
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: default
```

In our case, the `checksum-controller` reconciles `Secrets` in the `default` namespace and creates a `ConfigMap` including the secret data's checksums.
The created `ConfigMaps` are controlled by the respective `Secret`, i.e., there they have an `ownerReference` with `controller=true` to the `Secret`.

## The Sharder Webhook

The sharder created a `MutatingWebhookConfiguration` for the resources listed in our `ControllerRing` specification:

```bash
$ kubectl get mutatingwebhookconfiguration -l alpha.sharding.timebertt.dev/controllerring=checksum-controller
NAME                                 WEBHOOKS   AGE
controllerring-checksum-controller   1          71s
```

Let's examine the webhook configuration for more details.
We can see that the webhook targets a ring-specific path served by the `sharder`.
It reacts on `CREATE` and `UPDATE` requests of the configured resources, where the object doesn't have the ring-specific shard label.
I.e., it gets called for unassigned objects and adds the shard assignment label during admission.

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: controllerring-checksum-controller
webhooks:
- clientConfig:
    service:
      name: sharder
      namespace: sharding-system
      path: /webhooks/sharder/controllerring/checksum-controller
      port: 443
  name: sharder.sharding.timebertt.dev
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: default
  objectSelector:
    matchExpressions:
    - key: shard.alpha.sharding.timebertt.dev/checksum-controller
      operator: DoesNotExist
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - '*'
    operations:
    - CREATE
    - UPDATE
    resources:
    - secrets
    scope: '*'
  - apiGroups:
    - ""
    apiVersions:
    - '*'
    operations:
    - CREATE
    - UPDATE
    resources:
    - configmaps
    scope: '*'
```

## Creating Sharded Objects

We can observe the behavior of the webhook by creating a first example object.
When we create a `Secret`, the webhook assigns it to one of the available controller instances by adding the ring-specific shard label.
It performs a consistent hashing algorithm, where it hashes both the object's key (consisting of API group, `kind`, `namespace`, and `name`) and the shards' names onto a virtual ring.
It picks the shard with the hash value that is next to the object's hash clock-wise.

```bash
$ kubectl create secret generic foo --from-literal foo=bar -oyaml
apiVersion: v1
data:
  foo: YmFy
kind: Secret
metadata:
  labels:
    shard.alpha.sharding.timebertt.dev/checksum-controller: checksum-controller-c95c4fdb6-hv8pb
  name: foo
  namespace: default
type: Opaque
```

We can see that the responsible shard reconciled the `Secret` and created a `ConfigMap` for it.
Similar to the `Secret`, the `ConfigMap` was also assigned by the webhook.
In this case however, the sharder uses the information about the owning `Secret` for calculating the object's hash key.
With this, owned objects are always assigned to the same shard as their owner.
This is done because the controller typically needs to reconcile the owning object whenever the status of an owned object changes.
E.g., the `Deployment` controller watches `ReplicaSets` and continues rolling updates of the owning `Deployment` as soon as the owned `ReplicaSet` has the number of wanted replicas.

```bash
$ kubectl get configmap checksums-foo -oyaml
apiVersion: v1
data:
  foo: fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9
kind: ConfigMap
metadata:
  labels:
    shard.alpha.sharding.timebertt.dev/checksum-controller: checksum-controller-c95c4fdb6-hv8pb
  name: checksums-foo
  namespace: default
  ownerReferences:
  - apiVersion: v1
    controller: true
    kind: Secret
    name: foo
```

Let's create a few more `Secrets` and observe the distribution of objects across shards:

```bash
$ for i in $(seq 1 9); do kubectl create secret generic foo$i ; done
$ kubectl get secret,configmap -L shard.alpha.sharding.timebertt.dev/checksum-controller
NAME          TYPE     DATA   AGE   CHECKSUM-CONTROLLER
secret/foo    Opaque   1      39s   checksum-controller-c95c4fdb6-hv8pb
secret/foo1   Opaque   0      10s   checksum-controller-c95c4fdb6-rtvrm
secret/foo2   Opaque   0      10s   checksum-controller-c95c4fdb6-7jb2v
secret/foo3   Opaque   0      10s   checksum-controller-c95c4fdb6-rtvrm
secret/foo4   Opaque   0      10s   checksum-controller-c95c4fdb6-hv8pb
secret/foo5   Opaque   0      10s   checksum-controller-c95c4fdb6-rtvrm
secret/foo6   Opaque   0      10s   checksum-controller-c95c4fdb6-hv8pb
secret/foo7   Opaque   0      10s   checksum-controller-c95c4fdb6-7jb2v
secret/foo8   Opaque   0      10s   checksum-controller-c95c4fdb6-rtvrm
secret/foo9   Opaque   0      10s   checksum-controller-c95c4fdb6-hv8pb

NAME                         DATA   AGE     CHECKSUM-CONTROLLER
configmap/checksums-foo      1      39s     checksum-controller-c95c4fdb6-hv8pb
configmap/checksums-foo1     0      10s     checksum-controller-c95c4fdb6-rtvrm
configmap/checksums-foo2     0      10s     checksum-controller-c95c4fdb6-7jb2v
configmap/checksums-foo3     0      10s     checksum-controller-c95c4fdb6-rtvrm
configmap/checksums-foo4     0      10s     checksum-controller-c95c4fdb6-hv8pb
configmap/checksums-foo5     0      10s     checksum-controller-c95c4fdb6-rtvrm
configmap/checksums-foo6     0      10s     checksum-controller-c95c4fdb6-hv8pb
configmap/checksums-foo7     0      10s     checksum-controller-c95c4fdb6-7jb2v
configmap/checksums-foo8     0      10s     checksum-controller-c95c4fdb6-rtvrm
configmap/checksums-foo9     0      10s     checksum-controller-c95c4fdb6-hv8pb
```

## Removing Shards From the Ring

Let's see what happens when the set of available shards changes.
We can observe the actions that the sharder takes using `kubectl get secret --show-labels -w --output-watch-events --watch-only` in a new terminal session.

First, let's scale down the sharded controller to remove one shard from the ring:

```bash
TODO
$ kubectl scale deployment checksum-controller --replicas 2
deployment.apps/checksum-controller scaled
```

The shard releases its `Lease` by setting the `holderIdentity` field to the empty string.
The sharder recognizes that the shard was removed from the ring and sets its state to `dead`.
With this, the shard is no longer considered for object assignments.
The orphaned `Lease` is cleaned up after 1 minute.

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/state
NAME                                  HOLDER                                AGE     STATE
checksum-controller-c95c4fdb6-7jb2v   checksum-controller-c95c4fdb6-7jb2v   3m34s   ready
checksum-controller-c95c4fdb6-hv8pb   checksum-controller-c95c4fdb6-hv8pb   3m34s   ready
checksum-controller-c95c4fdb6-rtvrm                                         3m34s   dead
```

We can observe that the sharder immediately moved objects that were assigned to the removed shard to the remaining available shards.
For this, the sharder controller simply removes the shard label on all affected objects and lets the webhook reassign them.
As the original shard is not available anymore, moving the objects doesn't need to be coordinated and the sharder can immediately move objects.

```bash
$ kubectl get secret --show-labels -w --output-watch-events --watch-only
EVENT      NAME   TYPE     DATA   AGE   LABELS
MODIFIED   foo1   Opaque   0      48s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-hv8pb
MODIFIED   foo3   Opaque   0      48s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-7jb2v
MODIFIED   foo5   Opaque   0      48s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-hv8pb
MODIFIED   foo8   Opaque   0      48s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-7jb2v
```

## Adding Shards to the Ring

Now, let's scale up our sharded controller to add a new shard to the ring.

```bash
$ kubectl scale deployment checksum-controller --replicas 3
deployment.apps/checksum-controller scaled
```

We can observe that the new `Lease` object is in state `ready`.
With this, the new shard is immediately considered for assignment of new objects.

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/state
NAME                                  HOLDER                                AGE     STATE
checksum-controller-c95c4fdb6-7jb2v   checksum-controller-c95c4fdb6-7jb2v   4m19s   ready
checksum-controller-c95c4fdb6-hv8pb   checksum-controller-c95c4fdb6-hv8pb   4m19s   ready
checksum-controller-c95c4fdb6-kdrss   checksum-controller-c95c4fdb6-kdrss   4s      ready
```

In this case, a rebalancing needs to happen and the sharder needs to move objects away from available shards to the new shard.
In contrast to moving objects from unavailable shards, this needs to be coordinated to prevent multiple shards from acting on the same object concurrently.
Otherwise, the shards might perform conflicting actions which might lead to a broken state of the objects.

For this, the sharder adds the drain label to all objects that should be moved to the new shard.
This asks the currently responsible shard to stop reconciling the object and acknowledge the movement.
As soon as the controller observes the drain label, it removes it again along with the shard label.
This triggers the sharder webhook which immediately assigns the object to the desired shard.

```bash
$ kubectl get secret --show-labels -w --output-watch-events --watch-only
EVENT      NAME   TYPE     DATA   AGE    LABELS
MODIFIED   foo5   Opaque   0      116s   drain.alpha.sharding.timebertt.dev/checksum-controller=true,shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-hv8pb
MODIFIED   foo8   Opaque   0      116s   drain.alpha.sharding.timebertt.dev/checksum-controller=true,shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-7jb2v
MODIFIED   foo5   Opaque   0      116s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-kdrss
MODIFIED   foo8   Opaque   0      116s   shard.alpha.sharding.timebertt.dev/checksum-controller=checksum-controller-c95c4fdb6-kdrss
```

## Clean Up

Simply delete the local cluster to clean up:

```bash
make kind-down
```

## Where To Go From Here?

Now, you should have a basic understanding of how sharding for Kubernetes controllers works.
If you want to learn more about the individual components, the sharding architecture, and the reasoning behind it, see [Design](design.md).
You might also be interested in reading the [Evaluation](evaluation.md) document about load tests for sharded controllers and how this project helps in scaling Kubernetes controllers.

If you want to use sharding for your own controllers, see [Implement Sharding in Your Controller](implement-sharding.md).

To further experiment with the setup from this guide or to start developing changes to the sharding components, see [Development and Testing Setup](development.md).

You can also take a look at the remaining docs in the [documentation index](README.md).
