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

package errors

import (
	"fmt"
	"strings"
)

// FormatErrors is like multierror.ListFormatFunc without the noisy newlines and tabs.
// It also simplies the format for a single error.
func FormatErrors(es []error) string {
	if len(es) == 1 {
		return es[0].Error()
	}

	errs := make([]string, len(es))
	for i, err := range es {
		errs[i] = err.Error()
	}

	return fmt.Sprintf("%d errors occurred: %s", len(es), strings.Join(errs, ", "))
}
