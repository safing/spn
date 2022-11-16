package terminal

const (
	// UsePriorityDataMsgs defines whether priority data messages should be used.
	UsePriorityDataMsgs = true
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
		SubmitControl:       SubmitControlPlain,
		UsePriorityDataMsgs: UsePriorityDataMsgs,
	}
}

// DefaultExpansionTerminalOpts returns the default terminal options for an
// expansion terminal.
func DefaultExpansionTerminalOpts() *TerminalOpts {
	return &TerminalOpts{
		Padding:             8,
		FlowControl:         FlowControlDFQ,
		SubmitControl:       SubmitControlPlain,
		UsePriorityDataMsgs: UsePriorityDataMsgs,
	}
}
