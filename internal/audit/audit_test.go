package audit

import "testing"

func TestUserIDValueParsesNumericUserID(t *testing.T) {
	if got := userIDValue("147044"); got != uint64(147044) {
		t.Fatalf("expected numeric user id, got %#v", got)
	}
}

func TestUserIDValueReturnsNilForNonNumericUserID(t *testing.T) {
	if got := userIDValue("user-1"); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}
