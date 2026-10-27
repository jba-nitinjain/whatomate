package whatsapp

import (
	"errors"
	"fmt"
	"testing"
)

func TestParseMetaAPIErrorTyped(t *testing.T) {
	body := []byte(`{"error":{"message":"Invalid OAuth access token","code":190,"error_subcode":463}}`)
	err := ParseMetaAPIError(400, body)
	if err.Error() != "API error 190: Invalid OAuth access token" {
		t.Fatalf("unexpected message %q", err.Error())
	}
	var ge *GraphError
	if !errors.As(fmt.Errorf("wrapped: %w", err), &ge) {
		t.Fatal("expected GraphError through wrapping")
	}
	if ge.Code != 190 || ge.Subcode != 463 || ge.StatusCode != 400 {
		t.Fatalf("unexpected fields %+v", ge)
	}

	raw := ParseMetaAPIError(502, []byte("bad gateway"))
	if raw.Error() != "API returned status 502: bad gateway" {
		t.Fatalf("unexpected message %q", raw.Error())
	}
	if !errors.As(raw, &ge) || ge.StatusCode != 502 || ge.Code != 0 {
		t.Fatalf("unexpected fields %+v", ge)
	}
}
