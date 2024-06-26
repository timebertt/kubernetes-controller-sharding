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

package test

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
)

// RandomSuffix generates a random string that is safe to use as a name suffix in tests.
func RandomSuffix() string {
	unique := uuid.New()
	hash := sha256.Sum256(unique[:])
	return hex.EncodeToString(hash[:])[:8]
}
