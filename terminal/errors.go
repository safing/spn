package terminal

type Error string

const (
	// ErrNil represents a nil error.
	ErrNil Error = ""

	// ErrUnknownError is returned when an unknown error occurred.
	ErrUnknownError Error = "unknown"

	// ErrAbandoning is sent when the Hub is shutting down.
	ErrAbandoning Error = "abandoning"

	// ErrOpEnded is returned by ended operations.
	ErrOpEnded Error = "ended"

	// ErrMalformedData is returned when the request data was malformed and could not be parsed.
	ErrMalformedData Error = "malformed"

	// ErrUnknownOperation is returned when a requested command cannot be found.
	ErrUnknownOperationType Error = "optype"

	// ErrUnknownOperationID is returned when a requested command cannot be found.
	ErrUnknownOperationID Error = "opid"

	// ErrPermissinDenied is returned when calling a command with insufficient permissions.
	ErrPermissinDenied Error = "denied"

	// ErrQueueOverflow is returned when a full queue is encountered and data was discarded.
	ErrQueueOverflow Error = "overflow"

	// ErrUnexpectedMsgType is returned when an unexpected message type is received.
	ErrUnexpectedMsgType Error = "msgtype"

	// ErrIntegrity is returned when there is a data integrity error.
	ErrIntegrity Error = "integrity"

	// ErrCascading is returned when a connected component failed.
	ErrCascading Error = "cascading"

	// ErrInvalidConfiguration is returned when a component is configured
	// incorrectly.
	ErrInvalidConfiguration Error = "config"

	// ErrUnsupportedTerminalVersion is returned when an unsupported terminal
	// version is requested.
	ErrUnsupportedTerminalVersion Error = "terminal-version"

	// ErrInternalError is returned when an unspecified internal error occured.
	ErrInternalError Error = "internal"
)

// Error returns the human readable format of the error.
func (e Error) Error() string {
	switch e {
	case ErrNil:
		return "no error"
	case ErrUnknownError:
		return "unknown error"
	case ErrAbandoning:
		return "abandoning"
	case ErrOpEnded:
		return "operation ended"
	case ErrMalformedData:
		return "malformed data"
	case ErrUnknownOperationType:
		return "unknown operation type"
	case ErrUnknownOperationID:
		return "unknown operation id"
	case ErrPermissinDenied:
		return "permission denied"
	case ErrQueueOverflow:
		return "queue overflowed"
	case ErrUnexpectedMsgType:
		return "unexpected message type"
	case ErrIntegrity:
		return "integrity violated"
	case ErrCascading:
		return "cascading error"
	case ErrInvalidConfiguration:
		return "invalid configuration"
	case ErrUnsupportedTerminalVersion:
		return "unsupported terminal version"
	case ErrInternalError:
		return "internal error"
	default:
		return string(e) + " (no description)"
	}
}
