/*
Copyright 2023 Tim Ebert.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// Builder can build a sharded reconciler.
// Use NewShardedReconciler to create a new Builder.
type Builder struct {
	object          client.Object
	client          client.Client
	clusterRingName string
	shardName       string
	err             error
}

// NewShardedReconciler returns a new Builder for building a sharded reconciler.
// A sharded reconciler needs to handle the shard and drain labels correctly. This builder helps to construct a wrapper
// reconciler that takes care of the sharding-related logic and calls the delegate reconciler whenever the shard is
// responsible for reconciling an object.
func NewShardedReconciler(mgr manager.Manager) *Builder {
	return &Builder{
		client: mgr.GetClient(),
	}
}

// For sets the object kind being reconciled by the reconciler.
func (b *Builder) For(object client.Object) *Builder {
	if b.object != nil {
		b.err = fmt.Errorf("must not call For() more than once")
		return b
	}
	b.object = object
	return b
}

// WithClient overwrites the client to use for reading and patching the controller's objects.
// If not set, the manager's client is used.
func (b *Builder) WithClient(c client.Client) *Builder {
	b.client = c
	return b
}

// InClusterRing sets the name of the ClusterRing that the shard belongs to.
func (b *Builder) InClusterRing(name string) *Builder {
	b.clusterRingName = name
	return b
}

// WithShardName sets the name the shard.
func (b *Builder) WithShardName(name string) *Builder {
	b.shardName = name
	return b
}

// MustBuild calls Build and panics if Build returns an error.
func (b *Builder) MustBuild(r reconcile.Reconciler) reconcile.Reconciler {
	result, err := b.Build(r)
	utilruntime.Must(err)
	return result
}

// Build takes the actual reconciler and wraps it in the sharded reconciler.
func (b *Builder) Build(r reconcile.Reconciler) (reconcile.Reconciler, error) {
	if b.err != nil {
		return nil, b.err
	}
	if r == nil {
		return nil, fmt.Errorf("must provide a non-nil Reconciler")
	}
	if b.object == nil {
		return nil, fmt.Errorf("missing object kind, must call to For()")
	}
	if b.client == nil {
		return nil, fmt.Errorf("missing client")
	}
	if b.clusterRingName == "" {
		return nil, fmt.Errorf("missing ClusterRing name")
	}
	if b.shardName == "" {
		return nil, fmt.Errorf("missing shard name")
	}

	return &Reconciler{
		Object:     b.object,
		Client:     b.client,
		ShardName:  b.shardName,
		LabelShard: shardingv1alpha1.LabelShard(shardingv1alpha1.KindClusterRing, "", b.clusterRingName),
		LabelDrain: shardingv1alpha1.LabelDrain(shardingv1alpha1.KindClusterRing, "", b.clusterRingName),
		Do:         r,
	}, nil
}
