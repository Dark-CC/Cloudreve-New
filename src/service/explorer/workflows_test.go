package explorer

import "testing"

func TestSelectWorkflowNodeID(t *testing.T) {
	tests := []struct {
		name            string
		preferredNodeID string
		nodeID          string
		want            string
	}{
		{
			name:            "preferred takes precedence",
			preferredNodeID: "preferred",
			nodeID:          "legacy",
			want:            "preferred",
		},
		{
			name:   "legacy fallback",
			nodeID: "legacy",
			want:   "legacy",
		},
		{
			name: "empty means automatic",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := selectWorkflowNodeID(test.preferredNodeID, test.nodeID); got != test.want {
				t.Fatalf("selectWorkflowNodeID(%q, %q) = %q, want %q",
					test.preferredNodeID, test.nodeID, got, test.want)
			}
		})
	}
}
