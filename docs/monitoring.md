# Monitoring the Sharding Components

This document explains the metrics exposed by the sharder and sharding-exporter for monitoring the sharding components.

## sharder

The `sharder` service exposes metrics via `https` on the `8080` port at the `/metrics` endpoint.
Clients need to authenticate against the endpoint and must be authorized for `get` on the `nonResourceURL` `/metrics`.

### Exposed Metrics

#### `controller_sharding_assignments_total`

Type: counter  
Description: Total number of shard assignments by the sharder webhook per `ControllerRing` and GroupResource.
This counter is incremented every time the mutating webhook of the sharder assigns a sharded object (excluding dry-run requests).

#### `controller_sharding_movements_total`

Type: counter  
Description: Total number of shard movements triggered by the sharder controller per `ControllerRing` and GroupResource.
This counter is incremented every time the sharder controller triggers a direct object assignment, i.e., when an object needs to be moved away from an unavailable shard (or when an object has missed the webhook and needs to be assigned).
This only considers the sharder controller's side, i.e., the `controller_sharding_assignments_total` counter is incremented as well when the controller successfully triggers an assignment by the webhook.

#### `controller_sharding_drains_total`

Type: counter  
Description: Total number of shard drains triggered by the sharder controller per `ControllerRing` and GroupResource.
This counter is incremented every time the sharder controller triggers a drain operation, i.e., when an object needs to be moved away from an available shard.
This only considers the sharder controller's side, i.e., the `controller_sharding_assignments_total` counter is incremented as well when the shard removes the drain label as expect and thereby triggers an assignment by the webhook.
This doesn't consider the action taken by the shard.

#### `controller_sharding_ring_calculations_total`

Type: counter  
Description: Total number of hash ring calculations per `ControllerRing`.
This counter is incremented every time the sharder calculates a new consistent hash ring based on the shard leases.

## sharding-exporter

The sharding-exporter is an optional component for metrics about the state of `ControllerRings` and shards, see [Install the Sharding Components](installation.md#monitoring-optional).

The `sharding-exporter` service exposes metrics via `https` on the `8443` port at the `/metrics` endpoint.
Clients need to authenticate against the endpoint and must be authorized for `get` on the `nonResourceURL` `/metrics`.

### Exposed `ControllerRing` Metrics

#### `kube_controllerring_metadata_generation`

Type: gauge  
Description: The generation of a ControllerRing.

#### `kube_controllerring_observed_generation`

Type: gauge  
Description: The latest generation observed by the ControllerRing controller.

#### `kube_controllerring_status_shards`

Type: gauge  
Description: The ControllerRing's total number of shards observed by the ControllerRing controller.

#### `kube_controllerring_status_available_shards`

Type: gauge  
Description: The ControllerRing's number of available shards observed by the ControllerRing controller.

### Exposed Shard Metrics

#### `kube_shard_info`

Type: gauge  
Description: Information about a Shard.

#### `kube_shard_state`

Type: stateset  
Description: The Shard's current state observed by the shardlease controller.
