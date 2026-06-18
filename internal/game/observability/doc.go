// Package observability contains gameplay-domain command log and metric
// primitives for production readiness.
//
// Gameplay observability must stay separate from Symphony orchestration. This
// package must not import internal/symphony, create Symphony tasks, or mix
// agent workflow state into game telemetry.
package observability
