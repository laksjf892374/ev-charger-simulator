package entity

import "time"

type CommandKind string

const (
	CommandKindStartSession    CommandKind = "START_SESSION"
	CommandKindStopSession     CommandKind = "STOP_SESSION"
	CommandKindUnlockConnector CommandKind = "UNLOCK_CONNECTOR"
)

type CommandResult string

const (
	CommandResultAccepted        CommandResult = "ACCEPTED"
	CommandResultEVSEInoperative CommandResult = "EVSE_INOPERATIVE"
	CommandResultEVSEOccupied    CommandResult = "EVSE_OCCUPIED"
	CommandResultFailed          CommandResult = "FAILED"
	CommandResultTimeout         CommandResult = "TIMEOUT"
)

type CommandState string

const (
	CommandStatePending  CommandState = "PENDING"
	CommandStateRejected CommandState = "REJECTED"
	CommandStateResolved CommandState = "RESOLVED"
)

// Command is a remote request from an eMSP. It is answered synchronously (PENDING or REJECTED)
// and, if pending, resolved later with a Result.
type Command struct {
	AuthorizationReference string `json:"authorization_reference,omitempty"`
	// Opaque to the core; the adapter that accepted the command uses it to deliver the result.
	CallbackReference string        `json:"callback_reference,omitempty"`
	ChargerID         string        `json:"charger_id,omitempty"`
	CommandID         string        `json:"command_id"`
	CreatedAt         time.Time     `json:"created_at"`
	DeadlineAt        time.Time     `json:"deadline_at"`
	ForcedResult      CommandResult `json:"forced_result,omitempty"`
	Kind              CommandKind   `json:"kind"`
	Message           string        `json:"message,omitempty"`
	ReadyAt           time.Time     `json:"ready_at"`
	Result            CommandResult `json:"result,omitempty"`
	SessionID         string        `json:"session_id,omitempty"`
	State             CommandState  `json:"state"`
	Token             Token         `json:"token"`
	UpdatedAt         time.Time     `json:"updated_at"`
}
