package utils

import (
	"strings"
	"testing"
)

func TestGetPrivateServiceName(t *testing.T) {
	t.Run("stays within the 63-character Service limit for long names", func(t *testing.T) {
		long := "my-very-long-application-backend-service-v2-with-extra-suffix"
		got := GetPrivateServiceName(long)
		if len(got) > maxServiceNameLength {
			t.Errorf("name %q has length %d, exceeds limit %d", got, len(got), maxServiceNameLength)
		}
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("name %q does not start with prefix %q", got, prefix)
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		name := "my-very-long-application-backend-service-v2-with-extra-suffix"
		first, second := GetPrivateServiceName(name), GetPrivateServiceName(name)
		if first != second {
			t.Errorf("GetPrivateServiceName is not deterministic for %q: %q != %q", name, first, second)
		}
	})

	t.Run("distinguishes names sharing a truncated prefix via the hash", func(t *testing.T) {
		a := "my-very-long-application-backend-service-aaaaaaaaaaaaaaaaaaaa"
		b := "my-very-long-application-backend-service-bbbbbbbbbbbbbbbbbbbb"
		if GetPrivateServiceName(a) == GetPrivateServiceName(b) {
			t.Errorf("expected distinct names for %q and %q", a, b)
		}
	})

	t.Run("preserves short names unchanged", func(t *testing.T) {
		got := GetPrivateServiceName("svc")
		if got != prefix+"svc"+privateServicePostfix+"-"+got[len(got)-hashLength:] {
			t.Errorf("unexpected name for short input: %q", got)
		}
	})

	t.Run("does not leave a trailing hyphen before the postfix", func(t *testing.T) {
		// 41 chars then a hyphen at the truncation boundary; ensure no "--pvt".
		name := strings.Repeat("a", maxServiceNameLength) + "-" + strings.Repeat("b", 10)
		got := GetPrivateServiceName(name)
		if strings.Contains(got, "--"+strings.TrimPrefix(privateServicePostfix, "-")) {
			t.Errorf("name %q contains a doubled hyphen before postfix", got)
		}
		if len(got) > maxServiceNameLength {
			t.Errorf("name %q exceeds limit %d", got, maxServiceNameLength)
		}
	})
}

func TestGetEndpointSliceToResolverName(t *testing.T) {
	t.Run("stays within the 253-character EndpointSlice limit", func(t *testing.T) {
		long := strings.Repeat("a", 300)
		got := GetEndpointSliceToResolverName(long)
		if len(got) > maxEndpointSliceNameLength {
			t.Errorf("name %q has length %d, exceeds limit %d", got, len(got), maxEndpointSliceNameLength)
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		name := strings.Repeat("a", 300)
		first, second := GetEndpointSliceToResolverName(name), GetEndpointSliceToResolverName(name)
		if first != second {
			t.Errorf("GetEndpointSliceToResolverName is not deterministic: %q != %q", first, second)
		}
	})
}
