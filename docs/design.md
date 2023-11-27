# Design

This document explains the sharding design in more detail.
Please also consider reading the respective design chapters in the [study project](https://github.com/timebertt/thesis-controller-sharding) and [Master's thesis](https://github.com/timebertt/thesis-controller-sharding) as long as this document is not detailed enough.

## Architecture

This section outlines the key components and mechanism involved in achieving controller sharding.

![Sharding Architecture](assets/architecture.svg)

### The Sharder Component

The sharder is a central component deployed once per cluster.
It serves as the overall orchestrator of the sharding mechanism.
It facilitates membership and failure detection, partitioning, object assignment, and preventing concurrency.
The component is designed to be generic, i.e., it can be used for implementing sharding for any kind of controller (independent of the used programming language and controller framework).

### Shard Leases

Multiple instances of the actual controller are deployed.
Notably, no leader election is performed, and there is no designated single active instance.
Instead, each controller instance maintains an individual shard `Lease` labeled with the ring's name, allowing them to announce themselves to the sharder for membership and failure detection.
The sharder watches these leases to build a hash ring with the available instances.

### The `ClusterRing` Resource and Sharder Webhook

Rings of controllers are configured through the use of the `ClusterRing` custom resource.
The sharder creates a `MutatingWebhookConfiguration` for each `ClusterRing` to perform assignments for objects associated with the ring.
The sharder webhook is called on `CREATE` and `UPDATE` requests for configured resources, but only for objects that don't have the ring-specific shard label, i.e., for unassigned objects.

The sharder uses the consistent hashing ring to determine the desired shard and adds the shard label during admission accordingly.
Shards then use a label selector for the shard label with their own instance name to restrict the cache and controller to the subset of objects assigned to them.

For the controller's "main" object (configured in `ClusterRing.spec.resources[]`), the object's `apiVersion`, `kind`, `namespace`, and `name` are concatenated to form its hash key.
For objects controlled by other objects (configured in `ClusterRing.spec.resources[].controlledResources[]`), the sharder utilizes information about the controlling object (`ownerReference` with `controller=true`) to calculate the object's hash key.
This ensures that owned objects are consistently assigned to the same shard as their owner.

### Object Movements and Rebalancing

The sharder also runs a controller that facilitates object movements when necessary.
For this, it watches the shard leases and ensures all object assignments are up-to-date whenever the set of available instances changes.
It also performs periodic syncs to cater for objects that failed to be assigned during admission.

When a shard voluntarily releases its lease (i.e., on graceful shutdown), the sharder recognizes that the shard was removed from the ring and sets its state to `dead`.
With this, the shard is no longer considered for object assignments.
The orphaned `Lease` is cleaned up after 1 minute.
The sharder immediately moves objects that were assigned to the removed shard to the remaining available shards.
For this, the controller simply removes the shard label on all affected objects and lets the webhook reassign them.
As the original shard is not available anymore, moving the objects doesn't need to be coordinated and the sharder can immediately move objects.

When a shard fails to renew its lease in time, the sharder acquires the lease for ensuring API server reachability/functionality.
If this is successful, the shard is considered `dead` which leads to forcefully reassigning the objects.

When a new shard is added to the ring, the sharder recognizes the available shard lease and performs rebalancing accordingly.
In contrast to moving objects from unavailable shards, this needs to be coordinated to prevent multiple shards from acting on the same object concurrently.
Otherwise, the shards might perform conflicting actions which might lead to a broken state of the objects.

During rebalancing, the sharder drains objects from the old shard by adding the drain label.
This operation is acknowledged by the old shard by removing both the shard and the drain label
This in turn triggers the sharder webhook again, which assigns the object to the new shard.

## Important Design Decisions

### Don't Watch Sharded Objects

Distributing a controller's reconciliations and cache across multiple instances works very well using the label selector approach.
I.e., if you run 3 shards you can expect each shard to consume about a third of the CPU and memory consumption that a single instance responsible for all objects would.
The key to making Kubernetes controllers horizontally scalable however, is to ensure that the overhead of the sharding mechanism doesn't grow with the number of objects or rate of reconciliations.
Otherwise, we would only shift the scalability limitation to another component without removing it.
In other words, sharding Kubernetes controller obviously comes with an overhead – just as sharding a database.
However, this overhead needs to be constant or at maximum grow in a sublinear fashion.

In this project's [first iteration](https://github.com/timebertt/thesis-controller-sharding), the sharder didn't use a webhook to assign objects during admission.
Instead, the sharder ran a controller with watches for the sharded objects.
Although the sharder used lightweight metadata-only watches, the overhead still grew with the number of sharded objects.
In the study project's evaluation (see chapter 6 of the paper), it was shown that the setup was already capable of distributing resource consumption across multiple instances but still faced a scalability limitation in the sharder's resource consumption.

In the [second iteration](https://github.com/timebertt/masters-thesis-controller-sharding), the sharder doesn't watch the sharded objects anymore.
The watch events were only needed for labeling unassigned objects immediately.
This is facilitated by the sharder webhook instead now.
The other cases were object assignments need to be performed (membership changes) are unrelated to the objects themselves.
Hence, the controller only needs to watch a small number of objects related to the number of shards.
With this, the overhead of the sharding mechanism is independent of the number of objects.
In fact, it is negligible as show in [Evaluating the Sharding Mechanism](evaluation.md).
The comparisons show that the sharder's resource consumption is almost constant apart from spikes during periodic syncs.

### Minimize Impact on the Critical Path

While the use of mutating webhooks might allow dropping watches for the sharded objects, they can have a significant impact on API requests, e.g., regarding request latency.
To minimize the impact of the sharder's webhook on the overall request latency, the webhook is configured to only react on precisely the set of objects configured in the `ClusterRing` and only for `CREATE` and `UPDATE` requests of unassigned objects.
With this the webhook is only on the critical path during initial object creation and whenever the set of available shards requires reassignments.

Furthermore, webhooks can cause API requests to fail entirely.
To reduce the risk of such failures, the sharder is deployed in a highly available fashion and the webhook is configured with a low timeout and failure policy `Ignore`.
With this, API requests still succeed if the webhook server is shortly unreachable.
In such cases, the object will be unassigned until the next sync of the sharder controller.
I.e., the design prioritizes availability of the API over consistency of assignments.

Also, the sharding mechanism doesn't touch the critical path of actual reconciliations.

### Minimize Impact on the Control Plane

By using label selectors on the watch connections of individual shards, the load on the API server is not changed compared to a single controller instance that watches all objects without a selector.
Additionally, the sharder minimizes the extra load on the API server and etcd when it comes to `LIST` requests of all sharded objects (e.g., during periodic syncs).
For this, it only lists the metadata of the sharded objects (spec and status are irrelevant).
Also, it passes the [request parameter](https://kubernetes.io/docs/reference/using-api/api-concepts/#the-resourceversion-parameter) `resourceVersion=0` to the API server, which causes it to serve the request from the in-memory watch cache instead of performing a quorum read on etcd.
In other words, the design accepts slightly outdated data and with this slightly inconsistent object assignments in favor of better performance and scalability.

## Limitations

### Limited Support for `generateName`

In the first iteration (without the sharder webhook), the object's `uid` was the essential part of the hash key.
With the evolution of the mechanism to assign objects during admission using a mutating webhook, the object's `uid` cannot be used any longer as it is unset during admission for `CREATE` requests.
Hence, the sharder uses the object's `GroupVersionKind`, `namespace`, and `name` for calculating the hash key instead.
This works well and also supports calculating the same hash key for controlled objects by using information from `ownerReferences`.

However, this also means that `generateName` is not supported for resources that are not controlled by other resources in the ring.
The reason is that `generateName` is not set during admission for `CREATE` requests similar to the `uid` field.
Note, that `generateName` is still supported for objects that are controlled by other objects, as the controlled object's own name is not included in the hash key.

This tradeoff seems acceptable, as there are not many good use cases for `generateName`.
In general, using `generateName` in controllers makes it difficult to prevent incorrect actions (e.g., creating too many controlled objects) as the controller needs to track its own actions that used `generateName`.
Instead, using deterministic naming based on the owning object (e.g., spec contents or `uid`) simplifies achieving correctness significantly.
All other use cases of using `generateName` for simply generating a random name of an object one doesn't really care about (e.g., in integration or load tests) can also generate a random suffix on the client side before submitting the request to the API server.

However, if the API server set an object's `uid` or `generateName` before admission for `CREATE` requests, this limitation could be lifted.
