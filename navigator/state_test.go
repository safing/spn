package navigator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStates(t *testing.T) {
	t.Parallel()

	p := &Pin{}

	p.addStates(StateInvalid | StateFailing | StateSuperseded)
	assert.Equal(t, StateInvalid|StateFailing|StateSuperseded, p.State)

	p.removeStates(StateFailing | StateSuperseded)
	assert.Equal(t, StateInvalid, p.State)

	p.addStates(StateTrusted | StateActive)
	assert.True(t, p.State.has(StateInvalid|StateTrusted))
	assert.False(t, p.State.has(StateInvalid|StateSuperseded))
	assert.True(t, p.State.hasAnyOf(StateInvalid|StateTrusted))
	assert.True(t, p.State.hasAnyOf(StateInvalid|StateSuperseded))
	assert.False(t, p.State.hasAnyOf(StateSuperseded|StateFailing))

	assert.False(t, p.State.has(StateSummaryRegard))
	assert.False(t, p.State.has(StateSummaryDisregard))
	assert.True(t, p.State.hasAnyOf(StateSummaryRegard))
	assert.True(t, p.State.hasAnyOf(StateSummaryDisregard))
}
