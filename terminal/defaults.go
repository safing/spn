package terminal

import (
	"time"
)

const (
	// DefaultMediumPriorityMaxDelay defines the default maximum delay to use when
	// waiting for an execution slow when starting or signaling a microtask.
	// A connection carrying 1500B packets would still achieve over 100Mbit/s if
	// enough processing power and bandwidth is available.
	DefaultMediumPriorityMaxDelay = 100 * time.Microsecond

	// UsePriorityDataMsgs defines whether priority data messages should be used.
	UsePriorityDataMsgs = false
)

// DefaultCraneControllerOpts returns the default terminal options for a crane
// controller terminal.
func DefaultCraneControllerOpts() *TerminalOpts {
	return &TerminalOpts{
		Padding:             0, // Crane already applies padding.
		FlowControl:         FlowControlNone,
		SubmitControl:       SubmitControlPlain,
		UsePriorityDataMsgs: UsePriorityDataMsgs,
	}
}

// DefaultHomeHubTerminalOpts returns the default terminal options for a crane
// terminal used for the home hub.
func DefaultHomeHubTerminalOpts() *TerminalOpts {
	return &TerminalOpts{
		Padding:             0, // Crane already applies padding.
		FlowControl:         FlowControlDFQ,
		SubmitControl:       SubmitControlFair,
		UsePriorityDataMsgs: UsePriorityDataMsgs,
	}
}

// DefaultExpansionTerminalOpts returns the default terminal options for an
// expansion terminal.
func DefaultExpansionTerminalOpts() *TerminalOpts {
	return &TerminalOpts{
		Padding:             8,
		FlowControl:         FlowControlDFQ,
		SubmitControl:       SubmitControlFair,
		UsePriorityDataMsgs: UsePriorityDataMsgs,
	}
}
