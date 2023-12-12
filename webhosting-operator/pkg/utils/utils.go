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

// PickRandom picks a random element from the given slice.
func PickRandom[T any](in []T) T {
	return in[rand.Intn(len(in))]
}

// RandomName generates a random string with n characters that can be used as part of API object names.
func RandomName(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, n)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}
