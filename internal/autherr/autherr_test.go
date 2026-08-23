package autherr

import "testing"

func TestIs(t *testing.T) {
	base := New("missing credentials", nil)
	if !Is(base) {
		t.Fatal("expected CredentialsError to match")
	}
	wrapped := fmtWrap(base)
	if !Is(wrapped) {
		t.Fatal("expected wrapped CredentialsError to match")
	}
	if Is(nil) {
		t.Fatal("nil should not match")
	}
	if Is(plain("plain")) {
		t.Fatal("plain error should not match")
	}
}

type plain string

func (e plain) Error() string { return string(e) }

type wrap struct {
	msg   string
	cause error
}

func (w *wrap) Error() string { return w.msg }
func (w *wrap) Unwrap() error { return w.cause }

func fmtWrap(err error) error {
	return &wrap{msg: "outer: " + err.Error(), cause: err}
}
