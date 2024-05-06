// Copyright 2020 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_filterByLabels(t *testing.T) {
	assert.True(t, filterByLabels([]string{"label-a", "label-b", "label-c"}, []string{}))
	assert.True(t, filterByLabels([]string{"label-a", "label-b", "label-c"}, []string{"label-a"}))
	assert.True(t, filterByLabels([]string{"label-a", "label-b", "label-c"}, []string{"label-b"}))
	assert.True(t, filterByLabels([]string{"label-a", "label-b", "label-c"}, []string{"label-c"}))
	assert.False(t, filterByLabels([]string{"label-a", "label-b", "label-c"}, []string{"label-d"}))
}
