package inventory

import (
	"reflect"
	"testing"
)

func TestGiftCodeListWhere(t *testing.T) {
	used := true
	tests := []struct {
		name      string
		args      *GiftCodeListArgs
		wantWhere string
		wantArgs  []any
	}{
		{
			name:      "nil filter",
			wantWhere: "deleted_at IS NULL",
			wantArgs:  []any{},
		},
		{
			name: "all filters",
			args: &GiftCodeListArgs{
				Used:         &used,
				Code:         "exact-code",
				CodeContains: "partial",
				UserID:       42,
			},
			wantWhere: "deleted_at IS NULL AND used = ? AND code = ? AND used_by = ? AND LOWER(code) LIKE LOWER(?)",
			wantArgs:  []any{true, "exact-code", 42, "%partial%"},
		},
		{
			name: "unset optional filters",
			args: &GiftCodeListArgs{
				CodeContains: "only-search",
			},
			wantWhere: "deleted_at IS NULL AND LOWER(code) LIKE LOWER(?)",
			wantArgs:  []any{"%only-search%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := giftCodeListWhere(tt.args)
			if where != tt.wantWhere {
				t.Fatalf("where = %q, want %q", where, tt.wantWhere)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}

func TestGiftCodeListOrderWhitelist(t *testing.T) {
	tests := []struct {
		name          string
		args          *GiftCodeListArgs
		wantOrderBy   string
		wantDirection string
	}{
		{
			name:          "defaults",
			wantOrderBy:   "id",
			wantDirection: "DESC",
		},
		{
			name: "allowed column and direction",
			args: &GiftCodeListArgs{PaginationArgs: &PaginationArgs{
				OrderBy: "created_at",
				Order:   OrderDirectionAsc,
			}},
			wantOrderBy:   "created_at",
			wantDirection: "ASC",
		},
		{
			name: "untrusted column falls back",
			args: &GiftCodeListArgs{PaginationArgs: &PaginationArgs{
				OrderBy: "id; DROP TABLE gift_codes",
				Order:   OrderDirectionDesc,
			}},
			wantOrderBy:   "id",
			wantDirection: "DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderBy, direction := giftCodeListOrder(tt.args)
			if orderBy != tt.wantOrderBy || direction != tt.wantDirection {
				t.Fatalf("order = %q %q, want %q %q", orderBy, direction, tt.wantOrderBy, tt.wantDirection)
			}
		})
	}
}
