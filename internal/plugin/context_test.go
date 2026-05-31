package plugin

import (
	"errors"
	"testing"
)

func TestRejectErrorMapping(t *testing.T) {
	t.Run("errors.Is reject sentinel", func(t *testing.T) {
		err := Reject{Reason: "blocked"}
		if !errors.Is(err, ErrReject) {
			t.Fatal("expected errors.Is(err, ErrReject)")
		}
		if errors.Is(err, ErrAuthRequired) {
			t.Fatal("expected not auth required")
		}
	})

	t.Run("errors.As extracts reason", func(t *testing.T) {
		err := Reject{Reason: "duplicate"}
		var r Reject
		if !errors.As(err, &r) {
			t.Fatal("expected errors.As into Reject")
		}
		if r.Reason != "duplicate" {
			t.Fatalf("reason = %q, want duplicate", r.Reason)
		}
	})

	t.Run("empty reason uses sentinel message", func(t *testing.T) {
		err := Reject{}
		if err.Error() != ErrReject.Error() {
			t.Fatalf("Error() = %q, want %q", err.Error(), ErrReject.Error())
		}
	})
}

func TestAuthRequiredErrorMapping(t *testing.T) {
	t.Run("errors.Is auth required sentinel", func(t *testing.T) {
		err := AuthRequired{Reason: "subscribe requires authentication"}
		if !errors.Is(err, ErrAuthRequired) {
			t.Fatal("expected errors.Is(err, ErrAuthRequired)")
		}
		if errors.Is(err, ErrReject) {
			t.Fatal("expected not reject")
		}
	})

	t.Run("errors.As extracts reason", func(t *testing.T) {
		err := AuthRequired{Reason: "publish requires authentication"}
		var a AuthRequired
		if !errors.As(err, &a) {
			t.Fatal("expected errors.As into AuthRequired")
		}
		if a.Reason != "publish requires authentication" {
			t.Fatalf("reason = %q", a.Reason)
		}
	})

	t.Run("empty reason uses sentinel message", func(t *testing.T) {
		err := AuthRequired{}
		if err.Error() != ErrAuthRequired.Error() {
			t.Fatalf("Error() = %q, want %q", err.Error(), ErrAuthRequired.Error())
		}
	})
}

func TestRejectAndAuthRequiredDistinct(t *testing.T) {
	rejectErr := Reject{Reason: "no"}
	authErr := AuthRequired{Reason: "no"}
	if errors.Is(rejectErr, authErr) {
		t.Fatal("reject should not match auth required")
	}
}
