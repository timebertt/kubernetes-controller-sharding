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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// Predicate sets up a predicate that evaluates to true if
// a) the object needs to be drained or
// b) the given shard is responsible and all other predicates match.
// To support sharding in your controller, use this function to construct a predicate for the object kind given to
// `builder.Builder.For` (i.e., the controller's main kind).
// It is not needed to use this predicate for secondary watches (e.g., for object kinds given to
// `builder.Builder.{Owns,Watches}`) as secondary objects are not drained by the sharder.
func Predicate(clusterRingName, shardName string, predicates ...predicate.Predicate) predicate.Predicate {
	return predicate.Or(
		// always enqueue if we need to acknowledge the drain operation, other predicates don't matter in this case
		isDrained(clusterRingName),
		// or enqueue if we are responsible and all other predicates match
		predicate.And(isAssigned(clusterRingName, shardName), predicate.And(predicates...)),
	)
}

func isAssigned(clusterRingName, shardName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetLabels()[shardingv1alpha1.LabelShard(shardingv1alpha1.KindClusterRing, "", clusterRingName)] == shardName
	})
}

func isDrained(clusterRingName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		_, drain := object.GetLabels()[shardingv1alpha1.LabelDrain(shardingv1alpha1.KindClusterRing, "", clusterRingName)]
		return drain
	})
}
