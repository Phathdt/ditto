package errorx

import "errors"

// Variable with connection errors.
var (
	errReplConnectionIsLost = errors.New("replication connection to postgres is lost")
	errConnectionIsLost     = errors.New("db connection to postgres is lost")
	ErrMessageLost          = errors.New("messages are lost")
	ErrEmptyWALMessage      = errors.New("empty WAL message")
	ErrUnknownMessageType   = errors.New("unknown message type")
	ErrRelationNotFound     = errors.New("relation not found")
)

type serviceErr struct {
	Caller string
	Err    error
}

func (e *serviceErr) Error() string {
	return e.Caller + ": " + e.Err.Error()
}
