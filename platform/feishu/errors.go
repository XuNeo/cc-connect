package feishu

import (
	"errors"
	"fmt"
)

// feishuAPIError carries the Code/Msg from a non-success Feishu open-platform
// response so upstream code can recover based on the specific error.
type feishuAPIError struct {
	Code int
	Msg  string
}

func (e *feishuAPIError) Error() string {
	return fmt.Sprintf("feishu: code=%d msg=%s", e.Code, e.Msg)
}

// IsPermanent reports whether this error refers to a resource that is
// gone and will not come back. The writer uses this to decide whether
// to keep retrying or to degrade immediately.
func (e *feishuAPIError) IsPermanent() bool {
	switch e.Code {
	case 230011: // message withdrawn — target is gone, no recovery possible
		return true
	}
	return false
}

type feishuErrKind int

const (
	errKindOther feishuErrKind = iota
	errKindRateLimited
	errKindExpired
	errKindTooComplex
	errKindChatUnavailable
	errKindReplyTargetGone
)

// classifyFeishuError maps a wrapped *feishuAPIError to one of the known
// recovery categories. Anything else is errKindOther.
func classifyFeishuError(err error) feishuErrKind {
	var fe *feishuAPIError
	if !errors.As(err, &fe) {
		return errKindOther
	}
	switch fe.Code {
	case 230020:
		return errKindRateLimited
	case 230031:
		return errKindExpired
	case 230099, 200800:
		return errKindTooComplex
	case 230002:
		return errKindChatUnavailable
	case 230011:
		return errKindReplyTargetGone
	}
	return errKindOther
}

// wrapAPIError builds the canonical error chain for a non-success Feishu
// response: "<tag>: <op>: feishu: code=<N> msg=<M>" with a nested
// *feishuAPIError so classifyFeishuError can recover the code.
func wrapAPIError(tag, op string, code int, msg string) error {
	return fmt.Errorf("%s: %s: %w", tag, op, &feishuAPIError{Code: code, Msg: msg})
}
