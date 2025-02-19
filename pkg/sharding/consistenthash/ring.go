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

package consistenthash

import (
	"slices"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// Hash is a function computing a 64-bit digest.
type Hash func(data string) uint64

// DefaultHash is the default Hash used by Ring.
var DefaultHash Hash = xxhash.Sum64String

// DefaultTokensPerNode is the default number of virtual nodes per node.
const DefaultTokensPerNode = 100

// New creates a new hash ring with the given configuration and adds the given nodes.
// The given Hash (or DefaultHash if nil) is used to hash nodes and keys (strings).
// Each node is assigned tokensPerNode tokens (or DefaultTokensPerNode if <= 0) – aka. virtual nodes – for a more
// uniform key distribution.
func New(hash Hash, tokensPerNode int, initialNodes ...string) *Ring {
	if hash == nil {
		hash = DefaultHash
	}
	if tokensPerNode <= 0 {
		tokensPerNode = DefaultTokensPerNode
	}

	numTokens := len(initialNodes) * tokensPerNode
	r := &Ring{
		hash:          hash,
		tokensPerNode: tokensPerNode,

		tokens:      make([]uint64, 0, numTokens),
		tokenToNode: make(map[uint64]string, numTokens),
	}
	r.AddNodes(initialNodes...)
	return r
}

// Ring implements consistent hashing, aka. ring hash (not thread-safe).
// It hashes nodes and keys (strings) onto a ring of tokens. Keys are mapped to the next token (node) on the ring.
// Nodes cannot be removed. Instantiate a new Ring instead.
type Ring struct {
	hash          Hash
	tokensPerNode int

	tokens      []uint64
	tokenToNode map[uint64]string
}

// IsEmpty returns true if there are no nodes in this Ring.
func (r *Ring) IsEmpty() bool {
	return len(r.tokens) == 0
}

// AddNodes adds hash tokens for the given nodes to this Ring.
func (r *Ring) AddNodes(nodes ...string) {
	for _, node := range nodes {
		for i := 0; i < r.tokensPerNode; i++ {
			t := r.hash(node + strconv.FormatInt(int64(i), 10))
			r.tokens = append(r.tokens, t)
			r.tokenToNode[t] = node
		}
	}

	// sort all tokens on the ring for binary searches
	slices.Sort(r.tokens)
}

// Hash hashes the given key onto the ring of tokens and returns the node that belongs to the next token on the ring.
func (r *Ring) Hash(key string) string {
	if r.IsEmpty() {
		return ""
	}

	// Hash key and find the next virtual node on the ring
	h := r.hash(key)
	i, _ := slices.BinarySearch(r.tokens, h)

	// walked the whole ring, next virtual node is the first one
	if i == len(r.tokens) {
		i = 0
	}

	return r.tokenToNode[r.tokens[i]]
}
