package core

import (
	"errors"
	"testing"
)

type permanentErr struct{}

func (permanentErr) Error() string   { return "permanent" }
func (permanentErr) IsPermanent() bool { return true }

func TestClassifyWriterError_DefaultTransient(t *testing.T) {
	if k := classifyWriterError(errors.New("random upstream error")); k != writerErrTransient {
		t.Fatalf("default classify = %v, want transient", k)
	}
}

func TestClassifyWriterError_PermanentInterface(t *testing.T) {
	if k := classifyWriterError(permanentErr{}); k != writerErrPermanent {
		t.Fatalf("IsPermanent() wrapper = %v, want permanent", k)
	}
}

func TestClassifyWriterError_Nil(t *testing.T) {
	if k := classifyWriterError(nil); k != writerErrNone {
		t.Fatalf("nil = %v, want none", k)
	}
}
