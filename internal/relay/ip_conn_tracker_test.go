package relay

import "testing"

func TestIPConnTrackerAcquireRelease(t *testing.T) {
	t.Parallel()
	tr := newIPConnTracker()
	ip := "203.0.113.10"

	n, ok := tr.tryAcquire(ip, 2)
	if !ok || n != 1 {
		t.Fatalf("first acquire: n=%d ok=%v", n, ok)
	}
	n, ok = tr.tryAcquire(ip, 2)
	if !ok || n != 2 {
		t.Fatalf("second acquire: n=%d ok=%v", n, ok)
	}
	n, ok = tr.tryAcquire(ip, 2)
	if ok || n != 2 {
		t.Fatalf("third acquire should fail: n=%d ok=%v", n, ok)
	}
	if tr.openCount(ip) != 2 {
		t.Fatalf("openCount = %d, want 2", tr.openCount(ip))
	}

	tr.release(ip)
	if tr.openCount(ip) != 1 {
		t.Fatalf("after one release openCount = %d, want 1", tr.openCount(ip))
	}
	tr.release(ip)
	if tr.openCount(ip) != 0 {
		t.Fatalf("after second release openCount = %d, want 0", tr.openCount(ip))
	}

	n, ok = tr.tryAcquire(ip, 2)
	if !ok || n != 1 {
		t.Fatalf("re-acquire after drain: n=%d ok=%v", n, ok)
	}
}

func TestIPConnTrackerUnlimitedMax(t *testing.T) {
	t.Parallel()
	tr := newIPConnTracker()
	ip := "203.0.113.99"
	for i := 0; i < 5; i++ {
		if _, ok := tr.tryAcquire(ip, 0); !ok {
			t.Fatalf("acquire %d with max=0 should succeed", i+1)
		}
	}
	if tr.openCount(ip) != 5 {
		t.Fatalf("openCount = %d, want 5", tr.openCount(ip))
	}
}

func TestIPConnTrackerIndependentIPs(t *testing.T) {
	t.Parallel()
	tr := newIPConnTracker()
	if _, ok := tr.tryAcquire("1.1.1.1", 1); !ok {
		t.Fatal("acquire 1.1.1.1")
	}
	if _, ok := tr.tryAcquire("1.1.1.1", 1); ok {
		t.Fatal("expected 1.1.1.1 at cap")
	}
	if _, ok := tr.tryAcquire("2.2.2.2", 1); !ok {
		t.Fatal("acquire 2.2.2.2 should succeed independently")
	}
}
