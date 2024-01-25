/*
Copyright 2024 Tim Ebert.

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

package tracker

import (
	"cmp"
	"slices"
	"sync"
)

// tracker is a generic map that only allows manipulating keys via the set operation ("create or update").
// It synchronizes access to the map itself, but not to the contained values. Values need to synchronize concurrent
// access on their own.
// This is done to efficiently store multi-level tracking information in a concurrency safe way.
type tracker[K cmp.Ordered, V any] struct {
	lock   sync.RWMutex
	values map[K]*V
	new    func() *V
}

// sortedValues returns a slice of values in the tracker's map sorted by their respective keys.
func (t *tracker[K, V]) sortedValues() []*V {
	t.lock.RLock()
	defer t.lock.RUnlock()

	keys := make([]K, 0, len(t.values))
	for k := range t.values {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	values := make([]*V, 0, len(t.values))
	for _, k := range keys {
		values = append(values, t.values[k])
	}

	return values
}

// set adds a new key to the map and runs mutate on its value, or mutates the value of an existing key.
func (t *tracker[K, V]) set(key K, mutate func(v *V)) {
	// fast path for existing keys
	t.lock.RLock()
	if v, ok := t.values[key]; ok {
		defer t.lock.RUnlock()
		mutate(v)
		return
	}

	// slow path for new keys
	// upgrade lock for adding new keys
	t.lock.RUnlock()
	t.lock.Lock()
	defer t.lock.Unlock()

	// re-check for concurrent additions
	if v, ok := t.values[key]; ok {
		mutate(v)
		return
	}

	// add new key
	var v *V
	if t.new != nil {
		v = t.new()
	} else {
		v = new(V)
	}

	t.values[key] = v
	mutate(v)
}
