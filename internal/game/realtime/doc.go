// Package realtime defines the MVP JSON protocol contracts for live world
// sessions.
//
// Clients send intent envelopes. The server resolves authenticated session,
// player, world, visibility, and rate posture outside this package before any
// world mutation is allowed.
//
// Client-facing events must contain only filtered payloads. Internal worker
// events may carry richer state, but those details must be transformed before
// being wrapped in EventEnvelope and sent to a client.
//
// State changes should commit before realtime broadcasts. If a broadcast fails,
// clients reconcile from snapshots or queries instead of treating the failed
// broadcast as gameplay truth.
package realtime
