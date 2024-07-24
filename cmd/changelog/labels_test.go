package changelog

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
