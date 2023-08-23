/*
Copyright 2022 Tim Ebert.

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

package utils

import (
	"math/rand"
)

// CopyMap returns a new map with the same contents as the given map.
func CopyMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// PickRandom picks a random element from the given slice.
func PickRandom[T any](in []T) T {
	return in[rand.Intn(len(in))]
}

// PickNRandom picks n random elements from the given slice.
func PickNRandom[T any](in []T, n int) []T {
	if n <= 0 {
		return nil
	}
	if n >= len(in) {
		return in
	}

	rand.Shuffle(len(in), func(i, j int) {
		in[i], in[j] = in[j], in[i]
	})

	return in[:n]
}
