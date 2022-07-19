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
	"math"
	"testing"
)

func TestDistribution(t *testing.T) {
	ring := New(DefaultHash, 1000)

	numHosts := 10
	hosts := make([]string, numHosts)
	dist := make(map[string]float64, len(hosts))
	for i := range hosts {
		host := fmt.Sprintf("10.42.0.%d", i)
		hosts[i] = host
		ring.AddNode(host)
		dist[host] = 0
	}

	// fmt.Println("Virtual Nodes:")
	last := ring.tokens[len(ring.tokens)-1]
	for _, token := range ring.tokens {
		node := ring.tokenToNode[token]
		percentage := float64(token-last) / math.MaxUint64
		dist[node] += percentage

		// fmt.Printf("\t%016x (%.5f): %.5f -> %s\n", token, float64(token)/math.MaxUint64, percentage, node)
		last = token
	}

	fmt.Println("Nodes distribution:")
	for _, host := range hosts {
		fmt.Printf("\t%s: %.5f\n", host, dist[host])
	}
}
