// Package premium contains the Phase 10 premium entitlement MVP.
//
// The package is intentionally independent from market and auction code. It
// records entitlement and weekly-stock state, while all wallet mutation goes
// through the economy wallet service.
package premium
