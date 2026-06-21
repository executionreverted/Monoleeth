package foundation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorCodesStringifyAndSerializePredictably(t *testing.T) {
	tests := []struct {
		name string
		code Code
		want string
	}{
		{name: "unauthenticated", code: CodeUnauthenticated, want: "ERR_UNAUTHENTICATED"},
		{name: "forbidden", code: CodeForbidden, want: "ERR_FORBIDDEN"},
		{name: "not found", code: CodeNotFound, want: "ERR_NOT_FOUND"},
		{name: "invalid payload", code: CodeInvalidPayload, want: "ERR_INVALID_PAYLOAD"},
		{name: "rate limited", code: CodeRateLimited, want: "ERR_RATE_LIMITED"},
		{name: "internal", code: CodeInternal, want: "ERR_INTERNAL"},
		{name: "out of range", code: CodeOutOfRange, want: "ERR_OUT_OF_RANGE"},
		{name: "not visible", code: CodeNotVisible, want: "ERR_NOT_VISIBLE"},
		{name: "cooldown", code: CodeCooldown, want: "ERR_COOLDOWN"},
		{name: "not enough energy", code: CodeNotEnoughEnergy, want: "ERR_NOT_ENOUGH_ENERGY"},
		{name: "not enough cargo", code: CodeNotEnoughCargo, want: "ERR_NOT_ENOUGH_CARGO"},
		{name: "not enough funds", code: CodeNotEnoughFunds, want: "ERR_NOT_ENOUGH_FUNDS"},
		{name: "rank too low", code: CodeRankTooLow, want: "ERR_RANK_TOO_LOW"},
		{name: "item not tradeable", code: CodeItemNotTradeable, want: "ERR_ITEM_NOT_TRADEABLE"},
		{name: "ship disabled", code: CodeShipDisabled, want: "ERR_SHIP_DISABLED"},
		{name: "storage full", code: CodeStorageFull, want: "ERR_STORAGE_FULL"},
		{name: "pvp blocked", code: CodePVPBlocked, want: "ERR_PVP_BLOCKED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.code.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}

			payload, err := json.Marshal(tt.code)
			if err != nil {
				t.Fatalf("json marshal code: %v", err)
			}
			wantJSON := `"` + tt.want + `"`
			if got := string(payload); got != wantJSON {
				t.Fatalf("json = %s, want %s", got, wantJSON)
			}
		})
	}
}

func TestDomainErrorExposesPublicSafeMessage(t *testing.T) {
	err := NewDomainError(
		CodeNotVisible,
		"No valid signal found.",
		WithDetail("hidden planet planet-9 at 200,300 requires radar 4"),
	)

	if err.Code != CodeNotVisible {
		t.Fatalf("Code = %q, want %q", err.Code, CodeNotVisible)
	}
	if err.Message != "No valid signal found." {
		t.Fatalf("Message = %q, want safe message", err.Message)
	}

	public := err.Public()
	if public.Code != CodeNotVisible {
		t.Fatalf("Public().Code = %q, want %q", public.Code, CodeNotVisible)
	}
	if public.Message != "No valid signal found." {
		t.Fatalf("Public().Message = %q, want safe message", public.Message)
	}
	if got := err.InternalDetail(); got == "" {
		t.Fatal("InternalDetail() is empty, want diagnostic detail")
	}
	if got := err.Error(); strings.Contains(got, "hidden planet") || strings.Contains(got, "radar 4") {
		t.Fatalf("Error() leaked internal detail: %q", got)
	}
}

func TestDomainErrorPublicSerializationOmitsInternalDetails(t *testing.T) {
	cause := errors.New("database row player-123 failed")
	err := NewDomainError(
		CodeInternal,
		"Request failed.",
		WithDetail("sql: duplicate idempotency key request-456"),
		WithCause(cause),
	)

	payload, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("json marshal domain error: %v", marshalErr)
	}

	got := string(payload)
	for _, leaked := range []string{"duplicate", "request-456", "database", "player-123", "detail", "cause"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("public JSON leaked %q in %s", leaked, got)
		}
	}
	if got != `{"code":"ERR_INTERNAL","message":"Request failed."}` {
		t.Fatalf("public JSON = %s, want code and safe message only", got)
	}

	publicPayload, marshalErr := json.Marshal(err.Public())
	if marshalErr != nil {
		t.Fatalf("json marshal public error: %v", marshalErr)
	}
	if string(publicPayload) != string(payload) {
		t.Fatalf("Public() JSON = %s, want %s", publicPayload, payload)
	}
}

func TestDomainErrorUnwrapsCauseAndReportsCode(t *testing.T) {
	cause := errors.New("storage unavailable")
	domainErr := NewDomainError(CodeInternal, "Request failed.", WithCause(cause))
	wrapped := fmt.Errorf("command failed: %w", domainErr)

	if !errors.Is(domainErr, cause) {
		t.Fatal("errors.Is(domainErr, cause) = false, want true")
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("errors.Is(wrapped, cause) = false, want true")
	}

	var gotDomainErr *DomainError
	if !errors.As(wrapped, &gotDomainErr) {
		t.Fatal("errors.As(wrapped, *DomainError) = false, want true")
	}
	if gotDomainErr != domainErr {
		t.Fatal("errors.As returned a different DomainError")
	}

	if !IsCode(wrapped, CodeInternal) {
		t.Fatal("IsCode(wrapped, CodeInternal) = false, want true")
	}
	if IsCode(wrapped, CodeNotFound) {
		t.Fatal("IsCode(wrapped, CodeNotFound) = true, want false")
	}
	code, ok := CodeOf(wrapped)
	if !ok {
		t.Fatal("CodeOf(wrapped) ok = false, want true")
	}
	if code != CodeInternal {
		t.Fatalf("CodeOf(wrapped) = %q, want %q", code, CodeInternal)
	}
}
