package oneyunpan

import (
	"strings"

	"github.com/cloudreve/Cloudreve/v4/ent"
)

// UploadEndpointForPolicy returns the 1yunpan worker the edge must forward
// chunks to. The edge never derives it from client-supplied data, so a ticket
// can only ever reach the endpoint this deployment uses.
func UploadEndpointForPolicy(policy *ent.StoragePolicy) string {
	if policy != nil && policy.Settings != nil {
		if endpoint := strings.TrimSpace(policy.Settings.OneyunpanUploadEndpoint); endpoint != "" {
			return strings.TrimRight(endpoint, "/")
		}
	}
	return DefaultUploadEndpoint
}
