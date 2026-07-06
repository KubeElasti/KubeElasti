package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	prefix                = "elasti-"
	privateServicePostfix = "-pvt"
	endpointSlicePostfix  = "-endpointslice-to-resolver"

	// hashLength is the number of hex characters of the source name's hash
	// appended to generated names to keep them unique.
	hashLength = 10

	// maxServiceNameLength is the maximum length of a Kubernetes Service name
	// (an RFC 1035 label). The API server rejects longer names.
	maxServiceNameLength = 63
	// maxEndpointSliceNameLength is the maximum length of a Kubernetes
	// EndpointSlice name (an RFC 1123 subdomain).
	maxEndpointSliceNameLength = 253
)

// GetPrivateServiceName returns a private service name for a given public service name.
// It generates a hash of the public service name and appends it to the private service name.
// This decreases the chance of a naming collision; note the hash is deterministic, so the
// same public service name always yields the same private service name.
// The public service name is truncated as needed so the result never exceeds the Kubernetes
// 63-character Service name limit, while the hash (always derived from the full name) preserves
// uniqueness even when the name is truncated.
func GetPrivateServiceName(publicSVCName string) string {
	return buildName(publicSVCName, privateServicePostfix, maxServiceNameLength)
}

// GetEndpointSliceToResolverName returns an endpoint slice name for a given service name.
// As with GetPrivateServiceName, the source name is truncated so the result stays within the
// Kubernetes 253-character EndpointSlice name limit while remaining deterministic and unique.
func GetEndpointSliceToResolverName(serviceName string) string {
	return buildName(serviceName, endpointSlicePostfix, maxEndpointSliceNameLength)
}

// buildName constructs a deterministic, length-bounded name of the form
// "<prefix><name><postfix>-<hash>". The hash is always computed from the full, untruncated
// name so uniqueness is preserved even when name is shortened to fit maxLength.
func buildName(name, postfix string, maxLength int) string {
	hash := sha256.New()
	hash.Write([]byte(name))
	hashed := hex.EncodeToString(hash.Sum(nil))[:hashLength]

	// Fixed overhead is everything except the (possibly truncated) source name:
	// prefix + postfix + "-" + hash.
	overhead := len(prefix) + len(postfix) + 1 + hashLength
	maxNameLength := max(maxLength-overhead, 0)
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
		// Avoid leaving a trailing hyphen so the segment before the postfix stays tidy.
		name = strings.TrimRight(name, "-")
	}
	return prefix + name + postfix + "-" + hashed
}
