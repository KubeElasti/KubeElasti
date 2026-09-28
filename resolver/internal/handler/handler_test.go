package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/truefoundry/elasti/pkg/messages"
	"go.uber.org/zap"
)

// After proxying a request the resolver disables traffic for the host for
// a while, turning requests away with 408 so clients reconnect to the
// now-serving target directly. That only holds while the target serves: if
// it has scaled back to zero, requests belong to the resolver again, which
// must wake the target instead of refusing them.

type fakeHostManager struct{ enabled []string }

func (f *fakeHostManager) GetHost(*http.Request) (*messages.Host, error) { return nil, nil }
func (f *fakeHostManager) ScheduleDisableTrafficForHost(string)          {}
func (f *fakeHostManager) EnableTrafficForHost(host string)              { f.enabled = append(f.enabled, host) }

type fakeReadiness struct {
	active bool
	err    error
	calls  []string
}

func (f *fakeReadiness) CheckIfServiceEndpointSliceActive(ns, svc string) (bool, error) {
	f.calls = append(f.calls, ns+"/"+svc)
	return f.active, f.err
}

func disabledHost() *messages.Host {
	return &messages.Host{
		IncomingHost:   "app.team-a.svc.cluster.local",
		Namespace:      "team-a",
		SourceService:  "app",
		TargetService:  "app-pvt",
		TrafficAllowed: false,
	}
}

func newTestHandler(hm HostManager, r Readiness) *Handler {
	return NewHandler(&Params{Logger: zap.NewNop(), HostManager: hm, Readiness: r})
}

func TestTrafficSwitchedAwayWhileTargetServes(t *testing.T) {
	hm, r := &fakeHostManager{}, &fakeReadiness{active: true}

	if !newTestHandler(hm, r).trafficSwitchedAway(disabledHost()) {
		t.Fatal("request not turned away while the target serves traffic directly")
	}
	if len(hm.enabled) != 0 {
		t.Fatalf("traffic re-enabled for %v while the target serves", hm.enabled)
	}
}

func TestTrafficSwitchedAwayTargetScaledBackToZero(t *testing.T) {
	hm, r := &fakeHostManager{}, &fakeReadiness{active: false}

	if newTestHandler(hm, r).trafficSwitchedAway(disabledHost()) {
		t.Fatal("request turned away with 408 although the target is back at zero")
	}
	if len(r.calls) != 1 || r.calls[0] != "team-a/app-pvt" {
		t.Fatalf("readiness checked for %v, want [team-a/app-pvt] (the private service)", r.calls)
	}
	if len(hm.enabled) != 1 || hm.enabled[0] != "app.team-a.svc.cluster.local" {
		t.Fatalf("traffic re-enabled for %v, want the incoming host", hm.enabled)
	}
}

func TestTrafficSwitchedAwayWhenReadinessUnknown(t *testing.T) {
	hm, r := &fakeHostManager{}, &fakeReadiness{err: errors.New("apiserver unavailable")}

	if !newTestHandler(hm, r).trafficSwitchedAway(disabledHost()) {
		t.Fatal("request not turned away when readiness could not be checked (keep the previous behaviour)")
	}
	if len(hm.enabled) != 0 {
		t.Fatalf("traffic re-enabled for %v on a readiness error", hm.enabled)
	}
}

func TestTrafficSwitchedAwayNotWhileTrafficAllowed(t *testing.T) {
	hm, r := &fakeHostManager{}, &fakeReadiness{active: true}
	host := disabledHost()
	host.TrafficAllowed = true

	if newTestHandler(hm, r).trafficSwitchedAway(host) {
		t.Fatal("request turned away while traffic is allowed")
	}
	if len(r.calls) != 0 {
		t.Fatalf("readiness checked %v for an allowed host; want no API call on the hot path", r.calls)
	}
}
