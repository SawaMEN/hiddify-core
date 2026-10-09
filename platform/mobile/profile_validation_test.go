package mobile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
)

func registerValidation(t *testing.T, id string) {
	t.Helper()
	if err := BeginProfileValidation(id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { FinishProfileValidation(id) })
}

func TestValidateProfileAcceptedShapes(t *testing.T) {
	registerValidation(t, "valid-shapes")
	for _, content := range []string{
		`{"type":"direct","tag":"direct"}`,
		`[{"type":"direct","tag":"direct"}]`,
		`{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}`,
	} {
		if err := ValidateProfile("valid-shapes", content, `{}`); err != nil {
			t.Fatalf("%s: %v", content, err)
		}
	}
}

func TestValidateProfileRejectsInvalidSourceAndSettings(t *testing.T) {
	registerValidation(t, "invalid-source")
	for _, content := range []string{"", "not a configuration", `{"type":"direct"} {"type":"direct"}`, strings.Repeat("x", config.MaxConfigBytes+1)} {
		if err := ValidateProfile("invalid-source", content, `{}`); err == nil {
			t.Fatal("accepted invalid profile")
		}
	}
	if err := ValidateProfile("invalid-source", `{"type":"direct"}`, `{"use-xray-core-when-possible":"invalid"}`); err == nil {
		t.Fatal("accepted invalid option type")
	}
}

func TestValidationCancellationBeforeNativeEntryAndIsolation(t *testing.T) {
	registerValidation(t, "cancelled")
	registerValidation(t, "independent")
	CancelProfileValidation("cancelled")
	if err := ValidateProfile("cancelled", `{"type":"direct"}`, `{}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if err := ValidateProfile("independent", `{"type":"direct"}`, `{}`); err != nil {
		t.Fatal(err)
	}
}

func TestValidationRegistrationAndCleanup(t *testing.T) {
	if err := ValidateProfile("missing", `{"type":"direct"}`, `{}`); err == nil {
		t.Fatal("accepted unregistered validation")
	}
	registerValidation(t, "cleanup")
	if err := BeginProfileValidation("cleanup"); err == nil {
		t.Fatal("accepted duplicate registration")
	}
	FinishProfileValidation("cleanup")
	FinishProfileValidation("cleanup")
	if err := BeginProfileValidation("cleanup"); err != nil {
		t.Fatal(err)
	}
	if err := BeginProfileValidation(""); err == nil {
		t.Fatal("accepted empty ID")
	}
}

func TestValidationSessionLimit(t *testing.T) {
	for _, id := range []string{"limit-one", "limit-two", "limit-three", "limit-four"} {
		registerValidation(t, id)
	}
	if err := BeginProfileValidation("limit-five"); err == nil {
		FinishProfileValidation("limit-five")
		t.Fatal("accepted validation beyond the session limit")
	}
	FinishProfileValidation("limit-one")
	registerValidation(t, "limit-five")
}

func TestExpiredValidationCannotEnterParser(t *testing.T) {
	registerValidation(t, "expired")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	profileValidations.Lock()
	profileValidations.sessions["expired"].cancel()
	profileValidations.sessions["expired"] = &profileValidationSession{ctx: ctx, cancel: cancel}
	profileValidations.Unlock()
	if err := ValidateProfile("expired", `{"type":"direct"}`, `{}`); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}
