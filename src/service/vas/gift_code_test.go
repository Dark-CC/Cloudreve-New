package vas

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
)

func TestBuildGiftCodeListArgs(t *testing.T) {
	hasher, err := hashid.New("gift-code-test")
	if err != nil {
		t.Fatalf("create hash ID encoder: %v", err)
	}
	userHashID := hashid.EncodeUserID(hasher, 42)

	service := &ListGiftCodesService{
		Page:           3,
		PageSize:       25,
		OrderBy:        "created_at",
		OrderDirection: "asc",
		Conditions: map[string]string{
			"gift_code_used":    "true",
			"gift_code_code":    "exact-code",
			"gift_code_user_id": userHashID,
		},
		Searches: map[string]string{
			"gift_code_code": "partial",
		},
	}

	args, err := buildGiftCodeListArgs(service, hasher)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if args.Page != 2 || args.PageSize != 25 || args.OrderBy != "created_at" || args.Order != inventory.OrderDirectionAsc {
		t.Fatalf("unexpected pagination args: %#v", args.PaginationArgs)
	}
	if args.Used == nil || !*args.Used {
		t.Fatal("used filter was not parsed")
	}
	if args.Code != "exact-code" || args.CodeContains != "partial" || args.UserID != 42 {
		t.Fatalf("unexpected filters: %#v", args)
	}
}

func TestBuildGiftCodeListArgsAcceptsNumericUserID(t *testing.T) {
	hasher, err := hashid.New("gift-code-test")
	if err != nil {
		t.Fatalf("create hash ID encoder: %v", err)
	}
	service := &ListGiftCodesService{
		PageSize: 10,
		Conditions: map[string]string{
			"gift_code_user_id": "7",
		},
	}

	args, err := buildGiftCodeListArgs(service, hasher)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if args.UserID != 7 {
		t.Fatalf("user ID = %d, want 7", args.UserID)
	}
}

func TestBuildGiftCodeListArgsRejectsInvalidFilters(t *testing.T) {
	hasher, err := hashid.New("gift-code-test")
	if err != nil {
		t.Fatalf("create hash ID encoder: %v", err)
	}

	services := []*ListGiftCodesService{
		{PageSize: 10, Conditions: map[string]string{"gift_code_used": "not-a-bool"}},
		{PageSize: 10, Conditions: map[string]string{"gift_code_user_id": "not-an-id"}},
	}
	for _, service := range services {
		if _, err := buildGiftCodeListArgs(service, hasher); err == nil {
			t.Fatalf("expected invalid filter to be rejected: %#v", service.Conditions)
		}
	}
}
