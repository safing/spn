package navigator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStates(t *testing.T) {
	p := &Pin{}

	p.setStates(StateInvalid | StateFailing | StateSuperseded)
	assert.Equal(t, StateInvalid|StateFailing|StateSuperseded, p.State)

	p.unsetStates(StateFailing | StateSuperseded)
	assert.Equal(t, StateInvalid, p.State)

	p.setStates(StateTrusted | StateActive)
	assert.True(t, p.State.hasAllOf(StateInvalid|StateTrusted))
	assert.False(t, p.State.hasAllOf(StateInvalid|StateSuperseded))
	assert.True(t, p.State.hasAnyOf(StateInvalid|StateTrusted))
	assert.True(t, p.State.hasAnyOf(StateInvalid|StateSuperseded))
	assert.False(t, p.State.hasAnyOf(StateSuperseded|StateFailing))

	assert.False(t, p.State.hasAllOf(StateSummaryRegard))
	assert.False(t, p.State.hasAllOf(StateSummaryDisregard))
	assert.True(t, p.State.hasAnyOf(StateSummaryRegard))
	assert.True(t, p.State.hasAnyOf(StateSummaryDisregard))
}
