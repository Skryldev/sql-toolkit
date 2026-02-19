package db

import "context"

// useContextPackage resolves the actual context sentinel values so that
// errors.Is() works correctly in defaultMap.
func useContextPackage() {
	context_deadline_exceeded = context.DeadlineExceeded
	context_canceled = context.Canceled
}