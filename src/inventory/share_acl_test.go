package inventory

import "testing"

func TestShareACLEffectivePermissionPrecedence(t *testing.T) {
	read := SharePermission{CanList: true, CanRead: true}
	readWrite := SharePermission{CanList: true, CanRead: true, CanCreate: true}
	deny := SharePermission{}

	acl := &ShareACL{
		User: []UserSharePermission{
			{UserID: 7, Permission: deny},
		},
		Group: []GroupSharePermission{
			{GroupID: 2, Permission: readWrite},
			{GroupID: 4, Permission: deny},
		},
		Builtin: []BuiltinSharePermission{
			{Audience: ShareAudienceAnonymous, Permission: deny},
			{Audience: ShareAudienceLoggedIn, Permission: read},
			{Audience: ShareAudienceSameGroup, Permission: read},
			{Audience: ShareAudienceOtherGroup, Permission: deny},
		},
	}

	tests := []struct {
		name        string
		userID      int
		groupID     int
		ownerGroup  int
		anonymous   bool
		want        SharePermission
		wantMatched bool
	}{
		{
			name:        "explicit user rule wins",
			userID:      7,
			groupID:     2,
			ownerGroup:  2,
			want:        deny,
			wantMatched: true,
		},
		{
			name:        "explicit group rule wins",
			userID:      6,
			groupID:     2,
			ownerGroup:  2,
			want:        readWrite,
			wantMatched: true,
		},
		{
			name:        "logged in audience merges with other group audience",
			userID:      6,
			groupID:     3,
			ownerGroup:  2,
			want:        read,
			wantMatched: true,
		},
		{
			name:        "same group audience",
			userID:      6,
			groupID:     3,
			ownerGroup:  3,
			want:        read,
			wantMatched: true,
		},
		{
			name:        "logged in audience",
			userID:      6,
			groupID:     0,
			ownerGroup:  2,
			want:        read,
			wantMatched: true,
		},
		{
			name:        "anonymous audience",
			anonymous:   true,
			want:        deny,
			wantMatched: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, matched := acl.EffectivePermission(tc.userID, tc.groupID, tc.ownerGroup, tc.anonymous)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if got != tc.want {
				t.Fatalf("permission = %+v, want %+v", got, tc.want)
			}
		})
	}

	if _, matched := (*ShareACL)(nil).EffectivePermission(6, 2, 2, false); matched {
		t.Fatal("nil ACL must not match")
	}
}
