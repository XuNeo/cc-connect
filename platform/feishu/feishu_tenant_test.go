package feishu

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsTenantAccessTokenInvalid_Code2200(t *testing.T) {
	err := fmt.Errorf("patch message: %w", &feishuAPIError{Code: 2200, Msg: "check app tenant fail"})
	if !isTenantAccessTokenInvalid(err) {
		t.Fatal("code=2200 'check app tenant fail' should trigger token refresh retry")
	}
}

func TestIsTenantAccessTokenInvalid_Code99991663(t *testing.T) {
	err := fmt.Errorf("send message: %w", &feishuAPIError{Code: 99991663, Msg: "invalid access token"})
	if !isTenantAccessTokenInvalid(err) {
		t.Fatal("code=99991663 should still be detected (regression guard)")
	}
}

func TestIsTenantAccessTokenInvalid_UnrelatedError(t *testing.T) {
	err := errors.New("some random failure")
	if isTenantAccessTokenInvalid(err) {
		t.Fatal("arbitrary errors must not trigger token refresh")
	}
}
