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

package consistenthash

import (
	"fmt"
	"sort"

	"github.com/cespare/xxhash/v2"
)

// Hash is a function computing a 64-bit digest.
type Hash func(data []byte) uint64

// DefaultHash is the default Hash used by Ring.
var DefaultHash Hash = xxhash.Sum64

// DefaultTokensPerNode is the default number of virtual nodes per node.
const DefaultTokensPerNode = 100

// New creates a new hash ring.
func New(fn Hash, tokensPerNode int, initialNodes ...string) *Ring {
	if fn == nil {
		fn = DefaultHash
	}
	if tokensPerNode <= 0 {
		tokensPerNode = DefaultTokensPerNode
	}

	r := &Ring{
		hash:          fn,
		tokensPerNode: tokensPerNode,

		tokens:      make([]uint64, 0, len(initialNodes)*tokensPerNode),
		tokenToNode: make(map[uint64]string, len(initialNodes)),
	}
	r.AddNodes(initialNodes...)
	return r
}

// Ring implements consistent hashing, aka ring hash (not thread-safe).
// It hashes nodes and keys onto a ring of tokens. Keys are mapped to the next node on the ring.
type Ring struct {
	hash          Hash
	tokensPerNode int

	tokens      []uint64
	tokenToNode map[uint64]string
}

func (r *Ring) IsEmpty() bool {
	return len(r.tokens) == 0
}

func (r *Ring) AddNodes(nodes ...string) {
	for _, node := range nodes {
		for i := 0; i < r.tokensPerNode; i++ {
			t := r.hash([]byte(fmt.Sprintf("%s-%d", node, i)))
			r.tokens = append(r.tokens, t)
			r.tokenToNode[t] = node
		}
	}

	// sort all tokens on the ring for binary searches
	sort.Slice(r.tokens, func(i, j int) bool {
		return r.tokens[i] < r.tokens[j]
	})
}

func (r *Ring) Hash(key string) string {
	if r.IsEmpty() {
		return ""
	}

	// Hash key and walk the ring until we find the next virtual node
	h := r.hash([]byte(key))

	// binary search
	i := sort.Search(len(r.tokens), func(i int) bool {
		return r.tokens[i] >= h
	})

	// walked the whole ring
	if i == len(r.tokens) {
		i = 0
	}

	return r.tokenToNode[r.tokens[i]]
}
