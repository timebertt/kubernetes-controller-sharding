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

package routes

import (
	"net/http"
	"net/http/pprof"
)

// ProfilingHandlers is the set of profiling endpoints.
// This can be added to controller-runtime's metrics server via manager.Options.Metrics.ExtraHandlers.
var ProfilingHandlers = map[string]http.Handler{
	"/debug/pprof":         http.RedirectHandler("/debug/pprof/", http.StatusFound),
	"/debug/pprof/":        http.HandlerFunc(pprof.Index),
	"/debug/pprof/profile": http.HandlerFunc(pprof.Profile),
	"/debug/pprof/symbol":  http.HandlerFunc(pprof.Symbol),
	"/debug/pprof/trace":   http.HandlerFunc(pprof.Trace),
}
