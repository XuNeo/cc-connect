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

type feishuErrKind int

const (
	errKindOther feishuErrKind = iota
	errKindRateLimited
	errKindExpired
	errKindTooComplex
	errKindChatUnavailable
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
	}
	return errKindOther
}
