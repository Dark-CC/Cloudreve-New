package main

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
)

func TestRoundTrip(t *testing.T) {
	salt := "vA2QQ4u1k99S4FB2baCOWtErQ1y4DpSGd7XygYpLhanyPyu282rbMnJeBRxqpB74v3XorqY33TSDZtgnwND5amykqWrPLpt7GXZmQS1vSgpvZosxtLqph6C2qc1MBudz4o4bEvYNy15sXcRAbaMFBRFd9M1DwoneeBgFx0ywOYr9111HDsIyP9HEOjcAV54czxTLjC1hFyEvDWntA0lUpd9fyWjLJZG70ncSiOxhHlaIhtdBpYaSA8n1Ec1lj2Vy"
	encoder, err := hashid.New(salt)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 3, 54, 645} {
		encoded := hashid.EncodeShareID(encoder, id)
		decoded, err := encoder.Decode(encoded, hashid.ShareID)
		t.Logf("id=%d encoded=%q decoded=%d err=%v", id, encoded, decoded, err)
		if err != nil || decoded != id {
			t.Fatalf("round trip failed for %d", id)
		}
	}
}
