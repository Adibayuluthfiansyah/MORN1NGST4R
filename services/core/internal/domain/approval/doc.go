// Package approval is the approval record, its lifecycle and the consumption
// state machine (claim, launch check, denial and retry claim).
//
// It imports only the operation and actor domain packages (structure.md 5.6)
// and reads no clock: time is always an argument.
package approval
