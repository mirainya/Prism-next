package runtime

import "errors"

// ErrSubmissionRejected is set only at a boundary known to precede call
// creation. A transaction commit error must never carry this marker: its
// outcome may be unknown and an outbox worker could already submit the call.
var ErrSubmissionRejected = errors.New("gateway submission rejected before call creation")
