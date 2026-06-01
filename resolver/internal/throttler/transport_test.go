package throttler

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// reservedRefusedAddr returns a loopback address that refuses connections: it
// briefly binds a listener to obtain a free port and then closes it, so dials
// to the returned address fail with "connection refused" until something binds
// the port again.
func reservedRefusedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close the reservation listener: %v", err)
	}
	return addr
}

// TestDialBackOffHelper_RetriesConnectionRefusedThenConnects reproduces the
// scale-from-zero scenario: the target is not yet accepting connections (so the
// dial is refused), and starts serving shortly after. The dialer must retry the
// refused dials within its backoff window and ultimately connect, rather than
// failing fast (which surfaces as a 502 to the client).
func TestDialBackOffHelper_RetriesConnectionRefusedThenConnects(t *testing.T) {
	addr := reservedRefusedAddr(t)

	var wg sync.WaitGroup
	wg.Add(1)
	lnCh := make(chan net.Listener, 1)
	go func() {
		defer wg.Done()
		// Start serving after a delay so the first dials are refused.
		time.Sleep(150 * time.Millisecond)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			lnCh <- nil
			return
		}
		lnCh <- l
	}()

	bo := wait.Backoff{Duration: 20 * time.Millisecond, Factor: 1.2, Jitter: 0.1, Steps: 30}
	conn, err := dialBackOffHelper(context.Background(), "tcp", addr, bo, nil)
	wg.Wait()

	ln := <-lnCh
	if ln == nil {
		t.Skip("could not re-bind the reserved port; environment-dependent")
	}
	defer ln.Close()

	if err != nil {
		t.Fatalf("expected dial to succeed after retrying connection refused, got: %v", err)
	}
	if conn == nil {
		t.Fatal("expected a non-nil connection")
	}
	_ = conn.Close()
}

// TestDialBackOffHelper_ReturnsErrorWhenAlwaysRefused ensures the retry budget
// is bounded: when nothing ever listens, the dialer still gives up and returns
// an error within a reasonable time.
func TestDialBackOffHelper_ReturnsErrorWhenAlwaysRefused(t *testing.T) {
	addr := reservedRefusedAddr(t)

	bo := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.2, Jitter: 0.1, Steps: 3}
	start := time.Now()
	conn, err := dialBackOffHelper(context.Background(), "tcp", addr, bo, nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected an error when the connection is always refused")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dial retry budget was not bounded, took: %v", elapsed)
	}
}
