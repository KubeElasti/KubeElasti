package controller

import (
	"context"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/truefoundry/elasti/pkg/scaling"
	uberZap "go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// The scale-down watcher scales targets down, so with two operator pods (HA,
// or old and new during a rolling update) exactly one may run it: the
// leader. When the leader stops, the other pod takes over.
var _ = Describe("Scale-down watcher leader election", func() {
	newManager := func() ctrl.Manager {
		lease, renew, retry := 2*time.Second, 1*time.Second, 200*time.Millisecond
		m, err := ctrl.NewManager(cfg, ctrl.Options{
			Metrics:                       metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress:        "0",
			LeaderElection:                true,
			LeaderElectionID:              "scale-down-watcher-test",
			LeaderElectionNamespace:       "default",
			LeaderElectionReleaseOnCancel: true,
			LeaseDuration:                 &lease,
			RenewDeadline:                 &renew,
			RetryPeriod:                   &retry,
		})
		Expect(err).NotTo(HaveOccurred())
		return m
	}
	countingWatcher := func(runs *atomic.Int32) *scaling.ScaleDownWatcher {
		return scaling.NewScaleDownWatcher(uberZap.NewNop(), 50*time.Millisecond, func(context.Context) error {
			runs.Add(1)
			return nil
		})
	}

	It("runs only on the leader, and fails over when the leader stops", func() {
		var runsA, runsB atomic.Int32
		mgrA, mgrB := newManager(), newManager()
		Expect(mgrA.Add(countingWatcher(&runsA))).To(Succeed())
		Expect(mgrB.Add(countingWatcher(&runsB))).To(Succeed())

		ctxA, stopA := context.WithCancel(context.Background())
		defer stopA()
		go func() { _ = mgrA.Start(ctxA) }()
		Eventually(runsA.Load, 10*time.Second, 50*time.Millisecond).Should(BeNumerically(">", 0))

		ctxB, stopB := context.WithCancel(context.Background())
		defer stopB()
		go func() { _ = mgrB.Start(ctxB) }()
		Consistently(runsB.Load, 2*time.Second, 50*time.Millisecond).Should(BeZero())

		stopA()
		Eventually(runsB.Load, 10*time.Second, 50*time.Millisecond).Should(BeNumerically(">", 0))
	})
})
