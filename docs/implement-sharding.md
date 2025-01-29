# Implement Sharding in Your Controller

This guide walks you through implementing sharding for your own controller.
Prerequisite for using a sharded controller setup is to install the sharding components in the cluster, see [Install the Sharding Components](installation.md).

## Configuring the `ControllerRing`

After installing the sharding components, you can go ahead and configure a `ControllerRing` object for your controller.
For all controllers that you want to shard, configure the controller's main resource and the controlled resources in `ControllerRing.spec.resources`.

As an example, let's consider a subset of kube-controller-manager's controllers: `Deployment` and `ReplicaSet`.

- The `Deployment` controller reconciles the `deployments` resource and controls `replicasets`.
- The `ReplicaSet` controller reconciles the `replicaset` resource and controls `pods`.

The corresponding `ControllerRing` for the `Deployment` controller would need to be configured like this:

```yaml
apiVersion: sharding.timebertt.dev/v1alpha1
kind: ControllerRing
metadata:
  name: kube-controller-manager-deployment
spec:
  resources:
  - group: apps
    resource: deployments
    controlledResources:
    - group: apps
      resource: replicasets
```

Note that the `ControllerRing` name must not be longer than 63 characters because it is used as part of the shard and drain label key (see below).

To allow the sharder to reassign the sharded objects during rebalancing, we need to grant the corresponding permissions.
We need to grant these permissions explicitly depending on what is configured in the `ControllerRing`.
Otherwise, the sharder would basically require `cluster-admin` access.
For the above example, we would use these RBAC manifests:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: sharding:controllerring:kube-controller-manager
rules:
- apiGroups:
  - apps
  resources:
  - deployments
  - replicaset
  verbs:
  - list
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: sharding:controllerring:kube-controller-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: sharding:controllerring:kube-controller-manager
subjects:
- kind: ServiceAccount
  name: sharder
  namespace: sharding-system
```

## Implementation Changes

To support sharding in your Kubernetes controller, only three aspects need to be implemented:

- announce ring membership and shard health: maintain individual shard `Leases` instead of performing leader election on a single `Lease`
- only watch, cache, and reconcile objects assigned to the respective shard: add a shard-specific label selector to watches
- acknowledge object movements during rebalancing: remove the drain and shard label when the drain label is set and stop reconciling the object

[`pkg/shard`](../pkg/shard) contains reusable reference implementations for these aspects.
[`cmd/shard`](../cmd/shard) serves as an example shard implementation that shows how to put the pieces together in controllers based on [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
However, sharding can also be implemented in controllers that don't use controller-runtime or that are written in another programming language than Go.

The following sections outline the exact requirements that a sharded controller needs to fulfill and then show how to implement them in controllers based on controller-runtime.
Don't be scared by the long descriptions.
Implementing these aspects is simple (especially if reusing the helpers designed for controller-runtime controllers) and only needs to be done once.
The long descriptions just make sure the requirements are perfectly clear if you need to implement them yourself.

### Shard Lease

In short: ensure your shard maintains a `Lease` object like this and only runs its controllers as long as it holds the `Lease`:

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  labels:
    alpha.sharding.timebertt.dev/controllerring: my-controllerring
  name: my-operator-565df55f4b-5vwpj
  namespace: operator-system
spec:
  holderIdentity: my-operator-565df55f4b-5vwpj # needs to equal the Lease's name
  leaseDurationSeconds: 15 # pick whatever you would use for leader election as well
```

Most controllers already perform leader election using a central `Lease` lock object.
Only if the instance is elected as the leader, it is allowed to run the controllers.
If it fails to renew the `Lease` in time, another instance is allowed to acquire the `Lease` and can run the controllers.
Hence, an instance must not run any controllers when it looses its `Lease`.
In fact, most implementations exit the entire process when failing to renew the lock for safety.

On graceful termination (e.g., during a rolling update), the active leader may release the lock by setting the `holderIdentity` field of the `Lease` to the empty string.
This allows another instance to acquire the `Lease` immediately without waiting for it to expire, which helps in quick leadership handovers.

The same mechanisms apply to sharded controllers.
But instead of using a central `Lease` object for all instances, each instance acquires and maintains its own `Lease` object to announce itself to the sharder.
A shard may only run its controllers as long as it holds its shard `Lease`.
I.e., when it fails to renew the shard `Lease` in time, it also needs to stop all controllers.
Similar to usual leader election, a shard may release its own shard `Lease` on graceful termination by removing the `holderIdentity`.
This immediately triggers reassignments by the sharder to minimize the duration where no shard is acting on a subset of objects.

In essence, all the existing machinery for leader election can be reused for maintaining the shard `Lease` – that is, with two minor changes.
First, the shard `Lease` needs to be labelled with `alpha.sharding.timebertt.dev/controllerring=<controllerring-name>` to specify which `ControllerRing` the shard belongs to.
Second, the name of the shard `Lease` needs to match the `holderIdentity`.
By default, the instance's hostname is used for both values.
If the `holderIdentity` differs from the name, the sharder assumes that the shard is unavailable.

In controller-runtime, you can configure your shard to maintain its shard `Lease` as follows:

```go
package main

import (
	shardlease "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/lease"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func run() error {
	restConfig := config.GetConfigOrDie()

	shardLease, err := shardlease.NewResourceLock(restConfig, nil, shardlease.Options{
		ControllerRingName: "my-controllerring",
	})
	if err != nil {
		return err
	}

	mgr, err := manager.New(restConfig, manager.Options{
		// SHARD LEASE
		// Use manager's leader election mechanism for maintaining the shard lease.
		// With this, controllers will only run as long as manager holds the shard lease.
		// After graceful termination, the shard lease will be released.
		LeaderElection:                      true,
		LeaderElectionResourceLockInterface: shardLease,
		LeaderElectionReleaseOnCancel:       true,

		// other options ...
	})
	if err != nil {
		return err
	}

	// add controllers and start manager as usual ...

	return nil
}
```

Note that if you're using controller-runtime, the same manager instance cannot run sharded and non-sharded controllers as a manager can only run under a single resource lock (either leader election or shard lease).

### Filtered Watch Cache

In short: use the following label selector on watches for all sharded resources listed in the `ControllerRing`.

```text
shard.alpha.sharding.timebertt.dev/example: my-operator-565df55f4b-5vwpj
```

The sharder assigns all sharded objects by adding a shard label that is specific to the `ControllerRing` (resources could be part of multiple `ControllerRings`).
The shard label's key consists of the `shard.alpha.sharding.timebertt.dev/` prefix followed by the `ControllerRing` name.
As the key part after the `/` must not exceed 63 characters, the `ControllerRing` name must not be longer than 63 characters.
The shard label's value is the name of the shard, i.e., the name of the shard lease and the shard lease's `holderIdentity`.

Once you have determined the shard label key for your `ControllerRing`, use it as a selector on all watches that your controller starts for any of the sharded resources.
With this, the shard will only cache the objects assigned to it and the controllers will only reconcile these objects.

Note that when you use a label or field selector on a watch connection and the label or field changes so that the selector doesn't match anymore, the API server will emit a `DELETE` watch event.

In controller-runtime, you can configure your shard to only watch and reconcile assigned objects as follows.
This snippet works with controller-runtime v0.16 and v0.17, other versions might require deviating configuration.

```go
package main

import (
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func run() error {
	// ...
	
	mgr, err := manager.New(restConfig, manager.Options{
		// FILTERED WATCH CACHE
		Cache: cache.Options{
			// Configure cache to only watch objects that are assigned to this shard.
			// This shard only watches sharded objects, so we can configure the label selector on the cache's global level.
			// If your shard watches sharded objects as well as non-sharded objects, use cache.Options.ByObject to configure
			// the label selector on object level.
			DefaultLabelSelector: labels.SelectorFromSet(labels.Set{
				shardingv1alpha1.LabelShard("my-controllerring"): shardLease.Identity(),
			}),
		},

		// other options ...
	})
	
	// ...
}
```

### Acknowledge Drain Operations

In short: ensure your sharded controller acknowledges drain operations.
When the drain label like this is added by the sharder, the controller needs to remove both the shard and the drain label and stop reconciling the object.

```text
drain.alpha.sharding.timebertt.dev/example
```

When the sharder needs to move an object from an available shard to another shard for rebalancing, it first adds the drain label to instruct the currently responsible shard to stop reconciling the object.
The shard needs to acknowledge this operation, as the sharder must prevent concurrent reconciliations of the same object in multiple shards.
The drain label's key is specific to the `ControllerRing` and follows the same pattern as the shard label (see above).
The drain label's value is irrelevant, only the presence of the label is relevant.

Apart from changing the controller's business logic to first check the drain label, also ensure that the watch event filtering logic (predicates) always reacts on events with the drain label set independent of the controller's actual predicates.

In controller-runtime, you can reuse the helpers for constructing correct predicates and a wrapping reconciler that correctly implements the drain operation as follows:

```go
package controller

import (
	shardcontroller "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManager adds a controller to the manager.
// shardName must match the shard lease's name/identity.
func (r *Reconciler) AddToManager(mgr manager.Manager, controllerRingName, shardName string) error {
	// ACKNOWLEDGE DRAIN OPERATIONS
	// Use the shardcontroller package as helpers for:
	// - a predicate that triggers when the drain label is present (even if the actual predicates don't trigger)
	// - wrapping the actual reconciler a reconciler that handles the drain operation for us
	return builder.ControllerManagedBy(mgr).
		Named("example").
		For(&corev1.ConfigMap{}, builder.WithPredicates(shardcontroller.Predicate(controllerRingName, shardName, MyConfigMapPredicate()))).
		Owns(&corev1.Secret{}, builder.WithPredicates(MySecretPredicate())).
		Complete(
			shardcontroller.NewShardedReconciler(mgr).
				For(&corev1.ConfigMap{}). // must match the kind in For() above
				InControllerRing(controllerRingName).
				WithShardName(shardName).
				MustBuild(r),
		)
}
```
