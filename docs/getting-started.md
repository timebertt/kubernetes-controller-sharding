# Getting Started With Controller Sharding

This guide walks you through getting started with controller sharding in a local cluster.

It sets up the sharder and an example sharded controller so that you can see the components in action.
This is great for trying out the project for the first time and learning about the basic concepts.

## Set Up

Create a local cluster using [kind](https://kind.sigs.k8s.io/) and deploy all components:

```bash
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml
make deploy TAG=latest
```

The sharder is running in the `sharding-system` namespace and the example shard is deployed to the `default` namespace:

```bash
$ kubectl -n sharding-system get po
NAME                       READY   STATUS    RESTARTS   AGE
sharder-57889fcd8c-p2wxf   1/1     Running   0          44s
sharder-57889fcd8c-z6bm5   1/1     Running   0          44s
$ kubectl get po
NAME                    READY   STATUS    RESTARTS   AGE
shard-9c6678c9f-8jc5b   1/1     Running   0          45s
shard-9c6678c9f-v4bw2   1/1     Running   0          45s
shard-9c6678c9f-xntqc   1/1     Running   0          45s
```

## The `ControllerRing` and `Lease` Objects

We can see that the `ControllerRing` object is ready and reports 3 available shards out of 3 total shards:

```bash
$ kubectl get controllerring example
NAME      READY   AVAILABLE   SHARDS   AGE
example   True    3           3        64s
```

All shards announce themselves to the sharder by maintaining an individual `Lease` object with the `alpha.sharding.timebertt.dev/controllerring` label.
We can observe that the sharder recognizes all shards as available by looking at the `alpha.sharding.timebertt.dev/state` label:

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/controllerring,alpha.sharding.timebertt.dev/state
NAME                    HOLDER                  AGE   CONTROLLERRING   STATE
shard-9c6678c9f-8jc5b   shard-9c6678c9f-8jc5b   72s   example          ready
shard-9c6678c9f-v4bw2   shard-9c6678c9f-v4bw2   72s   example          ready
shard-9c6678c9f-xntqc   shard-9c6678c9f-xntqc   72s   example          ready
```

The `ControllerRing` object specifies which API resources should be sharded.
Optionally, it allows selecting the namespaces in which API resources are sharded:

```yaml
apiVersion: sharding.timebertt.dev/v1alpha1
kind: ControllerRing
metadata:
  name: example
spec:
  resources:
  - group: ""
    resource: configmaps
    controlledResources:
    - group: ""
      resource: secrets
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: default
```

In our case, the sharded controller reconciles `ConfigMaps` in the `default` namespace and creates a `Secret` including the configmap's name prefixed with `dummy-`.
The created `Secrets` are controlled by the respective `ConfigMap`, i.e., there they have an `ownerReference` with `controller=true` to the `ConfigMap`.

## The Sharder Webhook

The sharder created a `MutatingWebhookConfiguration` for the resources listed in our `ControllerRing` specification:

```bash
$ kubectl get mutatingwebhookconfiguration -l alpha.sharding.timebertt.dev/controllerring=example
NAME                        WEBHOOKS   AGE
sharding-50d858e0-example   1          2m50s
```

Let's examine the webhook configuration for more details.
We can see that the webhook targets a ring-specific path served by the `sharder`.
It reacts on `CREATE` and `UPDATE` requests of the configured resources, where the object doesn't have the ring-specific shard label.
I.e., it gets called for unassigned objects and adds the shard assignment label during admission.

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: sharding-50d858e0-example
webhooks:
- clientConfig:
    service:
      name: sharder
      namespace: sharding-system
      path: /webhooks/sharder/controllerring/example
      port: 443
  name: sharder.sharding.timebertt.dev
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: default
  objectSelector:
    matchExpressions:
    - key: shard.alpha.sharding.timebertt.dev/50d858e0-example
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
    - configmaps
    scope: '*'
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
```

## Creating Sharded Objects

We can observe the behavior of the webhook by creating a first example object.
When we create a `ConfigMap`, the webhook assigns it to one of the available controller instances by adding the ring-specific shard label.
It performs a consistent hashing algorithm, where it hashes both the object's key (consisting of `apiVersion`, `kind`, `namespace`, and `name`) and the shards' names onto a virtual ring.
It picks the shard with the hash value that is next to the object's hash clock-wise.

```bash
$ kubectl create cm foo -oyaml
apiVersion: v1
kind: ConfigMap
metadata:
  labels:
    shard.alpha.sharding.timebertt.dev/50d858e0-example: shard-9c6678c9f-8jc5b
  name: foo
  namespace: default
```

We can see that the responsible shard reconciled the `ConfigMap` and created a `Secret` for it.
Similar to the `ConfigMap`, the `Secret` was also assigned by the webhook.
In this case however, the sharder uses the information about the owning `ConfigMap` for calculating the object's hash key.
With this, owned objects are always assigned to the same shard as their owner.
This is done because the controller typically needs to reconcile the owning object whenever the status of an owned object changes.
E.g., the `Deployment` controller watches `ReplicaSets` and continues rolling updates of the owning `Deployment` as soon as the owned `ReplicaSet` has the number of wanted replicas.

```bash
$ kubectl get secret dummy-foo -oyaml
apiVersion: v1
kind: Secret
metadata:
  labels:
    shard.alpha.sharding.timebertt.dev/50d858e0-example: shard-9c6678c9f-8jc5b
  name: dummy-foo
  namespace: default
  ownerReferences:
  - apiVersion: v1
    controller: true
    kind: ConfigMap
    name: foo
```

Let's create a few more `ConfigMaps` and observe the distribution of objects across shards:

```bash
$ for i in $(seq 1 9); do k create cm foo$i ; done
$ kubectl get cm,secret -L shard.alpha.sharding.timebertt.dev/50d858e0-example
NAME                         DATA   AGE     50D858E0-EXAMPLE
configmap/foo                0      52s     shard-9c6678c9f-8jc5b
configmap/foo1               0      7s      shard-9c6678c9f-v4bw2
configmap/foo2               0      6s      shard-9c6678c9f-8jc5b
configmap/foo3               0      6s      shard-9c6678c9f-v4bw2
configmap/foo4               0      6s      shard-9c6678c9f-v4bw2
configmap/foo5               0      6s      shard-9c6678c9f-xntqc
configmap/foo6               0      6s      shard-9c6678c9f-xntqc
configmap/foo7               0      6s      shard-9c6678c9f-xntqc
configmap/foo8               0      6s      shard-9c6678c9f-xntqc
configmap/foo9               0      6s      shard-9c6678c9f-8jc5b

NAME                            TYPE     DATA   AGE     50D858E0-EXAMPLE
secret/dummy-foo                Opaque   0      52s     shard-9c6678c9f-8jc5b
secret/dummy-foo1               Opaque   0      7s      shard-9c6678c9f-v4bw2
secret/dummy-foo2               Opaque   0      6s      shard-9c6678c9f-8jc5b
secret/dummy-foo3               Opaque   0      6s      shard-9c6678c9f-v4bw2
secret/dummy-foo4               Opaque   0      6s      shard-9c6678c9f-v4bw2
secret/dummy-foo5               Opaque   0      6s      shard-9c6678c9f-xntqc
secret/dummy-foo6               Opaque   0      6s      shard-9c6678c9f-xntqc
secret/dummy-foo7               Opaque   0      6s      shard-9c6678c9f-xntqc
secret/dummy-foo8               Opaque   0      6s      shard-9c6678c9f-xntqc
secret/dummy-foo9               Opaque   0      6s      shard-9c6678c9f-8jc5b
```

## Removing Shards From the Ring

Let's see what happens when the set of available shards changes.
We can observe the actions that the sharder takes using `kubectl get cm --show-labels -w --output-watch-events --watch-only` in a new terminal session.

First, let's scale down the sharded controller to remove one shard from the ring:

```bash
$ kubectl scale deployment shard --replicas 2
deployment.apps/shard scaled
```

The shard releases its `Lease` by setting the `holderIdentity` field to the empty string.
The sharder recognizes that the shard was removed from the ring and sets its state to `dead`.
With this, the shard is no longer considered for object assignments.
The orphaned `Lease` is cleaned up after 1 minute.

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/controllerring,alpha.sharding.timebertt.dev/state
NAME                    HOLDER                  AGE   CONTROLLERRING   STATE
shard-9c6678c9f-f49zn                           25s   example          dead
shard-9c6678c9f-kvgft   shard-9c6678c9f-kvgft   25s   example          ready
shard-9c6678c9f-ppzf7   shard-9c6678c9f-ppzf7   25s   example          ready
```

We can observe that the sharder immediately moved objects that were assigned to the removed shard to the remaining available shards.
For this, the sharder controller simply removes the shard label on all affected objects and lets the webhook reassign them.
As the original shard is not available anymore, moving the objects doesn't need to be coordinated and the sharder can immediately move objects.

```bash
$ kubectl get cm --show-labels -w --output-watch-events --watch-only
EVENT      NAME   DATA   AGE     LABELS
MODIFIED   foo4   0      7m52s   shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-ppzf7
MODIFIED   foo6   0      7m52s   shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-ppzf7
```

## Adding Shards to the Ring

Now, let's scale up our sharded controller to add a new shard to the ring.

```bash
$ kubectl scale deployment shard --replicas 3
deployment.apps/shard scaled
```

We can observe that the new `Lease` object is in state `ready`.
With this, the new shard is immediately considered for assignment of new objects.

```bash
$ kubectl get lease -L alpha.sharding.timebertt.dev/controllerring,alpha.sharding.timebertt.dev/state
NAME                    HOLDER                  AGE   CONTROLLERRING   STATE
shard-9c6678c9f-jkgj6   shard-9c6678c9f-jkgj6   3s    example          ready
shard-9c6678c9f-kvgft   shard-9c6678c9f-kvgft   96s   example          ready
shard-9c6678c9f-ppzf7   shard-9c6678c9f-ppzf7   96s   example          ready
```

In this case, a rebalancing needs to happen and the sharder needs to move objects away from available shards to the new shard.
In contrast to moving objects from unavailable shards, this needs to be coordinated to prevent multiple shards from acting on the same object concurrently.
Otherwise, the shards might perform conflicting actions which might lead to a broken state of the objects.

For this, the sharder adds the drain label to all objects that should be moved to the new shard.
This asks the currently responsible shard to stop reconciling the object and acknowledge the movement.
As soon as the controller observes the drain label, it removes it again along with the shard label.
This triggers the sharder webhook which immediately assigns the object to the desired shard.

```bash
$ kubectl get cm --show-labels -w --output-watch-events --watch-only
EVENT      NAME   DATA   AGE    LABELS
MODIFIED   foo4   0      9m2s   drain.alpha.sharding.timebertt.dev/50d858e0-example=true,shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-ppzf7
MODIFIED   foo7   0      9m2s   drain.alpha.sharding.timebertt.dev/50d858e0-example=true,shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-ppzf7
MODIFIED   foo4   0      9m2s   shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-jkgj6
MODIFIED   foo7   0      9m2s   shard.alpha.sharding.timebertt.dev/50d858e0-example=shard-9c6678c9f-jkgj6
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
