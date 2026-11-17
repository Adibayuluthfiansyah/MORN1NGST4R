// Package actor is the kind and identity of whoever calls a use case (P-02).
//
// An Actor is human, system or ai. Authorization checks, approvals and human
// verification read the kind, which is how "AI cannot approve" is enforced
// instead of agreed. The package imports only the standard library.
package actor
