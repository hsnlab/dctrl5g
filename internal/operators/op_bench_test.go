package operators

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/l7mp/dcontroller/pkg/object"

	"github.com/hsnlab/dctrl5g/internal/dctrl"
	"github.com/hsnlab/dctrl5g/internal/testsuite"
)

func initBenchSuite(b *testing.B, ctx context.Context) {
	ctrl.SetLogger(logger.WithName("dctrl5g-bench"))
	d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
		{Name: "amf", File: "amf.yaml"},
		{Name: "ausf", File: "ausf.yaml"},
		{Name: "smf", File: "smf.yaml"},
		{Name: "pcf", File: "pcf.yaml"},
		{Name: "upf", File: "upf.yaml"},
	}, 0, 0)
	if err != nil {
		b.Fatalf("failed to start operators: %v", err)
	}
	logger = d.GetLogger()

	c = d.GetCache().GetClient()
	if c == nil {
		b.Fatal("failed to get client")
	}

	timeout = time.Second * 20
	interval = time.Millisecond * 50
}

// BenchmarkRegistration benchmarks the registration process by creating multiple
// registration requests in a tight loop and waiting for each to complete.
func BenchmarkRegistration(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Track created registrations for cleanup.
	var createdRegs []object.Object

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		// Reuse the same SUCI as uniqueness is not checked.
		name := fmt.Sprintf("bench-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// Create and wait for registration to be ready.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}

		createdRegs = append(createdRegs, reg)
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Cleanup: delete all created registrations.
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}
}

// BenchmarkRegistrationWithMemStats benchmarks registration with detailed memory statistics.
func BenchmarkRegistrationWithMemStats(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Track created registrations for cleanup.
	var createdRegs []object.Object

	// Force GC and get baseline memory stats.
	runtime.GC()
	var memStatsBefore, memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		// Reuse the same SUCI as uniqueness is not checked.
		name := fmt.Sprintf("bench-mem-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// Create and wait for registration to be ready.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}

		createdRegs = append(createdRegs, reg)
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Get memory stats after benchmark.
	runtime.ReadMemStats(&memStatsAfter)

	// Calculate memory used.
	totalAlloc := memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc
	heapAlloc := memStatsAfter.HeapAlloc - memStatsBefore.HeapAlloc
	numGC := memStatsAfter.NumGC - memStatsBefore.NumGC

	b.Logf("\n=== Memory Statistics ===")
	b.Logf("Total registrations: %d", b.N)
	b.Logf("Total allocated: %d bytes (%.2f MB)", totalAlloc, float64(totalAlloc)/(1024*1024))
	b.Logf("Per registration: %d bytes (%.2f MB)", totalAlloc/uint64(b.N), float64(totalAlloc/uint64(b.N))/(1024*1024))
	b.Logf("Heap allocated: %d bytes (%.2f MB)", heapAlloc, float64(heapAlloc)/(1024*1024))
	b.Logf("GC runs: %d", numGC)
	b.Logf("Mallocs: %d", memStatsAfter.Mallocs-memStatsBefore.Mallocs)
	b.Logf("Frees: %d", memStatsAfter.Frees-memStatsBefore.Frees)
	b.Logf("Live objects: %d", (memStatsAfter.Mallocs-memStatsBefore.Mallocs)-(memStatsAfter.Frees-memStatsBefore.Frees))

	// Cleanup: delete all created registrations.
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}
}

// BenchmarkRegistrationMemoryGrowth tracks memory growth over multiple registrations.
func BenchmarkRegistrationMemoryGrowth(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl.SetLogger(logger.WithName("dctrl5g-bench"))
	d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
		{Name: "amf", File: "amf.yaml"},
		{Name: "ausf", File: "ausf.yaml"},
		{Name: "smf", File: "smf.yaml"},
		{Name: "pcf", File: "pcf.yaml"},
		{Name: "upf", File: "upf.yaml"},
	}, 0, 0)
	if err != nil {
		b.Fatalf("failed to start operators: %v", err)
	}
	logger = d.GetLogger()

	c = d.GetCache().GetClient()
	if c == nil {
		b.Fatal("failed to get client")
	}

	// Track created registrations for cleanup.
	var createdRegs []object.Object

	// Force GC and get baseline.
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineHeap := memStats.HeapAlloc

	b.Logf("\n=== Memory Growth Tracking ===")
	b.Logf("Baseline heap: %d bytes (%.2f MB)", baselineHeap, float64(baselineHeap)/(1024*1024))

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Track memory every N operations.
	sampleInterval := 1
	if b.N > 10 {
		sampleInterval = b.N / 10
	}

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		name := fmt.Sprintf("bench-growth-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// Create and wait for registration to be ready.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}

		createdRegs = append(createdRegs, reg)

		// Sample memory at intervals.
		if (i+1)%sampleInterval == 0 {
			runtime.ReadMemStats(&memStats)
			currentHeap := memStats.HeapAlloc
			growth := int64(currentHeap) - int64(baselineHeap)
			perReg := growth / int64(i+1)
			b.Logf("After %d regs: heap=%d bytes (%.2f MB), growth=%.2f MB, per-reg=%.2f KB",
				i+1,
				currentHeap,
				float64(currentHeap)/(1024*1024),
				float64(growth)/(1024*1024),
				float64(perReg)/1024)
		}
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Final memory check.
	runtime.ReadMemStats(&memStats)
	finalHeap := memStats.HeapAlloc
	totalGrowth := int64(finalHeap) - int64(baselineHeap)

	b.Logf("\n=== Final Memory Report ===")
	b.Logf("Final heap: %d bytes (%.2f MB)", finalHeap, float64(finalHeap)/(1024*1024))
	b.Logf("Total growth: %.2f MB", float64(totalGrowth)/(1024*1024))
	b.Logf("Average per registration: %.2f KB", float64(totalGrowth)/float64(b.N)/1024)

	// Cleanup: delete all created registrations.
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}

	// Check memory after cleanup.
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	afterCleanup := memStats.HeapAlloc
	b.Logf("After cleanup: %d bytes (%.2f MB), leaked: %.2f MB",
		afterCleanup,
		float64(afterCleanup)/(1024*1024),
		float64(int64(afterCleanup)-int64(baselineHeap))/(1024*1024))
}

// BenchmarkRegistrationParallel benchmarks the registration process with parallel
// goroutines to test concurrent registration handling.
func BenchmarkRegistrationParallel(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Counter for unique registration IDs across all parallel goroutines.
	var regCounter int

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark in parallel.
	b.RunParallel(func(pb *testing.PB) {
		var localRegs []object.Object

		for pb.Next() {
			// Generate unique ID using counter.
			regCounter++
			i := regCounter

			// Create unique name and namespace for each registration.
			// Reuse the same SUCI as uniqueness is not checked.
			name := fmt.Sprintf("bench-parallel-user-%d", i)
			namespace := name
			suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

			// Create and wait for registration to be ready.
			reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
			if err != nil {
				b.Fatalf("failed to initialize registration %d: %v", i, err)
			}

			localRegs = append(localRegs, reg)
		}

		// Cleanup local registrations.
		for _, reg := range localRegs {
			if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
				b.Logf("warning: failed to delete registration %s/%s: %v",
					reg.GetNamespace(), reg.GetName(), err)
			}
		}
	})

	b.StopTimer()
}

// BenchmarkSession benchmarks the session establishment process.
func BenchmarkSession(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Track created objects for cleanup.
	var createdRegs []object.Object
	var createdSessions []object.Object

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		// Reuse the same SUCI as uniqueness is not checked.
		name := fmt.Sprintf("bench-session-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// First create a registration.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}
		createdRegs = append(createdRegs, reg)

		// Extract GUTI from the registration status.
		status, ok := reg.UnstructuredContent()["status"].(map[string]any)
		if !ok {
			b.Fatalf("failed to get status from registration %d", i)
		}
		guti, ok := status["guti"].(string)
		if !ok {
			b.Fatalf("failed to get GUTI from registration %d", i)
		}

		// Create session.
		session, err := initSessionErr(ctx, name, namespace, guti, i, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize session %d: %v", i, err)
		}
		createdSessions = append(createdSessions, session)
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Cleanup: delete sessions first, then registrations.
	for _, session := range createdSessions {
		if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete session %s/%s: %v",
				session.GetNamespace(), session.GetName(), err)
		}
	}
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}
}

// BenchmarkSessionWithMemStats benchmarks session creation with detailed memory statistics.
func BenchmarkSessionWithMemStats(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Track created objects for cleanup.
	var createdRegs []object.Object
	var createdSessions []object.Object

	// Force GC and get baseline memory stats.
	runtime.GC()
	var memStatsBefore, memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		// Reuse the same SUCI as uniqueness is not checked.
		name := fmt.Sprintf("bench-session-mem-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// First create a registration.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}
		createdRegs = append(createdRegs, reg)

		// Extract GUTI from the registration status.
		status, ok := reg.UnstructuredContent()["status"].(map[string]any)
		if !ok {
			b.Fatalf("failed to get status from registration %d", i)
		}
		guti, ok := status["guti"].(string)
		if !ok {
			b.Fatalf("failed to get GUTI from registration %d", i)
		}

		// Create session.
		session, err := initSessionErr(ctx, name, namespace, guti, i, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize session %d: %v", i, err)
		}
		createdSessions = append(createdSessions, session)
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Get memory stats after benchmark.
	runtime.ReadMemStats(&memStatsAfter)

	// Calculate memory used.
	totalAlloc := memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc
	heapAlloc := memStatsAfter.HeapAlloc - memStatsBefore.HeapAlloc
	numGC := memStatsAfter.NumGC - memStatsBefore.NumGC

	b.Logf("\n=== Session Memory Statistics ===")
	b.Logf("Total sessions (reg+session): %d", b.N)
	b.Logf("Total allocated: %d bytes (%.2f MB)", totalAlloc, float64(totalAlloc)/(1024*1024))
	b.Logf("Per session flow: %d bytes (%.2f MB)", totalAlloc/uint64(b.N), float64(totalAlloc/uint64(b.N))/(1024*1024))
	b.Logf("Heap allocated: %d bytes (%.2f MB)", heapAlloc, float64(heapAlloc)/(1024*1024))
	b.Logf("GC runs: %d", numGC)
	b.Logf("Mallocs: %d", memStatsAfter.Mallocs-memStatsBefore.Mallocs)
	b.Logf("Frees: %d", memStatsAfter.Frees-memStatsBefore.Frees)
	b.Logf("Live objects: %d", (memStatsAfter.Mallocs-memStatsBefore.Mallocs)-(memStatsAfter.Frees-memStatsBefore.Frees))
	b.Logf("\nNote: Each iteration includes both registration AND session creation")

	// Cleanup: delete sessions first, then registrations.
	for _, session := range createdSessions {
		if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete session %s/%s: %v",
				session.GetNamespace(), session.GetName(), err)
		}
	}
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}
}

// BenchmarkSessionMemoryGrowth tracks memory growth over multiple session creations.
func BenchmarkSessionMemoryGrowth(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Track created objects for cleanup.
	var createdRegs []object.Object
	var createdSessions []object.Object

	// Force GC and get baseline.
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineHeap := memStats.HeapAlloc

	b.Logf("\n=== Session Memory Growth Tracking ===")
	b.Logf("Baseline heap: %d bytes (%.2f MB)", baselineHeap, float64(baselineHeap)/(1024*1024))

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Track memory every N operations.
	sampleInterval := 1
	if b.N > 10 {
		sampleInterval = b.N / 10
	}

	// Run benchmark.
	for i := 0; i < b.N; i++ {
		// Create unique name and namespace for each registration.
		name := fmt.Sprintf("bench-session-growth-user-%d", i)
		namespace := name
		suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"

		// First create a registration.
		reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize registration %d: %v", i, err)
		}
		createdRegs = append(createdRegs, reg)

		// Extract GUTI from the registration status.
		status, ok := reg.UnstructuredContent()["status"].(map[string]any)
		if !ok {
			b.Fatalf("failed to get status from registration %d", i)
		}
		guti, ok := status["guti"].(string)
		if !ok {
			b.Fatalf("failed to get GUTI from registration %d", i)
		}

		// Create session.
		session, err := initSessionErr(ctx, name, namespace, guti, i, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to initialize session %d: %v", i, err)
		}
		createdSessions = append(createdSessions, session)

		// Sample memory at intervals.
		if (i+1)%sampleInterval == 0 {
			runtime.ReadMemStats(&memStats)
			currentHeap := memStats.HeapAlloc
			growth := int64(currentHeap) - int64(baselineHeap)
			perSession := growth / int64(i+1)
			b.Logf("After %d sessions: heap=%d bytes (%.2f MB), growth=%.2f MB, per-session=%.2f KB",
				i+1,
				currentHeap,
				float64(currentHeap)/(1024*1024),
				float64(growth)/(1024*1024),
				float64(perSession)/1024)
		}
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Final memory check.
	runtime.ReadMemStats(&memStats)
	finalHeap := memStats.HeapAlloc
	totalGrowth := int64(finalHeap) - int64(baselineHeap)

	b.Logf("\n=== Final Session Memory Report ===")
	b.Logf("Final heap: %d bytes (%.2f MB)", finalHeap, float64(finalHeap)/(1024*1024))
	b.Logf("Total growth: %.2f MB", float64(totalGrowth)/(1024*1024))
	b.Logf("Average per session flow: %.2f KB", float64(totalGrowth)/float64(b.N)/1024)
	b.Logf("Note: Each flow includes registration + session creation")

	// Cleanup: delete sessions first, then registrations.
	for _, session := range createdSessions {
		if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete session %s/%s: %v",
				session.GetNamespace(), session.GetName(), err)
		}
	}
	for _, reg := range createdRegs {
		if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
			b.Logf("warning: failed to delete registration %s/%s: %v",
				reg.GetNamespace(), reg.GetName(), err)
		}
	}

	// Check memory after cleanup.
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	afterCleanup := memStats.HeapAlloc
	b.Logf("After cleanup: %d bytes (%.2f MB), leaked: %.2f MB",
		afterCleanup,
		float64(afterCleanup)/(1024*1024),
		float64(int64(afterCleanup)-int64(baselineHeap))/(1024*1024))
}

// BenchmarkTransition benchmarks the active->idle->active transition process.
// Creates a single registration+session pair, then repeatedly performs the transition cycle.
func BenchmarkTransition(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Create a single registration and session for all iterations.
	name := "bench-transition-user"
	namespace := name
	suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
	sessionId := 1

	// Create registration.
	reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize registration: %v", err)
	}

	// Extract GUTI from registration.
	status, ok := reg.UnstructuredContent()["status"].(map[string]any)
	if !ok {
		b.Fatal("failed to get status from registration")
	}
	guti, ok := status["guti"].(string)
	if !ok {
		b.Fatal("failed to get GUTI from registration")
	}

	// Create session.
	session, err := initSessionErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize session: %v", err)
	}

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark - repeatedly transition the same session.
	for i := 0; i < b.N; i++ {
		// Transition to idle: create ContextRelease.
		ctxRel, err := initContextReleaseErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to create context release %d: %v", i, err)
		}

		// Transition back to active: delete ContextRelease.
		if err := c.Delete(ctx, ctxRel); err != nil && !apierrors.IsNotFound(err) {
			b.Fatalf("failed to delete context release %d: %v", i, err)
		}

		// Wait for UPF Config to reappear (indicating active state).
		upfConfig := object.NewViewObject("upf", "Config")
		object.SetName(upfConfig, namespace, name)

		ticker := time.NewTicker(interval)
		timeoutTimer := time.NewTimer(timeout)
		configReady := false

	loop:
		for {
			select {
			case <-timeoutTimer.C:
				ticker.Stop()
				b.Fatalf("timeout waiting for UPF config to reappear for iteration %d", i)
			case <-ticker.C:
				if err := c.Get(ctx, client.ObjectKeyFromObject(upfConfig), upfConfig); err == nil {
					configReady = true
					break loop
				}
			}
		}
		ticker.Stop()
		timeoutTimer.Stop()

		if !configReady {
			b.Fatalf("UPF config did not reappear for iteration %d", i)
		}
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Cleanup: delete session and registration.
	if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete session %s/%s: %v",
			session.GetNamespace(), session.GetName(), err)
	}
	if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete registration %s/%s: %v",
			reg.GetNamespace(), reg.GetName(), err)
	}
}

// BenchmarkTransitionWithMemStats benchmarks transitions with detailed memory statistics.
// Creates a single registration+session pair, then repeatedly performs the transition cycle.
func BenchmarkTransitionWithMemStats(b *testing.B) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Create a single registration and session for all iterations.
	name := "bench-trans-mem-user"
	namespace := name
	suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
	sessionId := 1

	// Create registration.
	reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize registration: %v", err)
	}

	// Extract GUTI from registration.
	status, ok := reg.UnstructuredContent()["status"].(map[string]any)
	if !ok {
		b.Fatal("failed to get status from registration")
	}
	guti, ok := status["guti"].(string)
	if !ok {
		b.Fatal("failed to get GUTI from registration")
	}

	// Create session.
	session, err := initSessionErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize session: %v", err)
	}

	// Force GC and get baseline memory stats.
	runtime.GC()
	var memStatsBefore, memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Run benchmark - repeatedly transition the same session.
	for i := 0; i < b.N; i++ {
		// Transition to idle.
		ctxRel, err := initContextReleaseErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to create context release %d: %v", i, err)
		}

		// Transition back to active.
		if err := c.Delete(ctx, ctxRel); err != nil && !apierrors.IsNotFound(err) {
			b.Fatalf("failed to delete context release %d: %v", i, err)
		}

		// Wait for UPF Config to reappear.
		upfConfig := object.NewViewObject("upf", "Config")
		object.SetName(upfConfig, namespace, name)

		ticker := time.NewTicker(interval)
		timeoutTimer := time.NewTimer(timeout)
		configReady := false

	loopMem:
		for {
			select {
			case <-timeoutTimer.C:
				ticker.Stop()
				b.Fatalf("timeout waiting for UPF config to reappear for iteration %d", i)
			case <-ticker.C:
				if err := c.Get(ctx, client.ObjectKeyFromObject(upfConfig), upfConfig); err == nil {
					configReady = true
					break loopMem
				}
			}
		}
		ticker.Stop()
		timeoutTimer.Stop()

		if !configReady {
			b.Fatalf("UPF config did not reappear for iteration %d", i)
		}
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Get memory stats after benchmark.
	runtime.ReadMemStats(&memStatsAfter)

	// Calculate memory used.
	totalAlloc := memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc
	heapAlloc := memStatsAfter.HeapAlloc - memStatsBefore.HeapAlloc
	numGC := memStatsAfter.NumGC - memStatsBefore.NumGC

	b.Logf("\n=== Transition Memory Statistics ===")
	b.Logf("Total transitions: %d", b.N)
	b.Logf("Total allocated: %d bytes (%.2f MB)", totalAlloc, float64(totalAlloc)/(1024*1024))
	b.Logf("Per transition: %d bytes (%.2f MB)", totalAlloc/uint64(b.N), float64(totalAlloc/uint64(b.N))/(1024*1024))
	b.Logf("Heap allocated: %d bytes (%.2f MB)", heapAlloc, float64(heapAlloc)/(1024*1024))
	b.Logf("GC runs: %d", numGC)
	b.Logf("Mallocs: %d", memStatsAfter.Mallocs-memStatsBefore.Mallocs)
	b.Logf("Frees: %d", memStatsAfter.Frees-memStatsBefore.Frees)
	b.Logf("Live objects: %d", (memStatsAfter.Mallocs-memStatsBefore.Mallocs)-(memStatsAfter.Frees-memStatsBefore.Frees))
	b.Logf("\nNote: Each iteration is idle->active transition only (reg+session creation excluded)")

	// Cleanup: delete session and registration.
	if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete session %s/%s: %v",
			session.GetNamespace(), session.GetName(), err)
	}
	if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete registration %s/%s: %v",
			reg.GetNamespace(), reg.GetName(), err)
	}
}

// BenchmarkTransitionMemoryGrowth tracks memory growth over multiple transitions.
// Creates a single registration and session pair, then repeatedly performs only the transition cycle.
func BenchmarkTransitionMemoryGrowth(b *testing.B) {
	// Setup.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initBenchSuite(b, ctx)

	// Create single registration and session pair.
	name := "bench-trans-growth-user"
	namespace := name
	suci := "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
	sessionId := 1

	// Create registration.
	reg, err := initRegErr(ctx, name, namespace, suci, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize registration: %v", err)
	}

	// Extract GUTI from registration.
	status, ok := reg.UnstructuredContent()["status"].(map[string]any)
	if !ok {
		b.Fatalf("failed to get status from registration")
	}
	guti, ok := status["guti"].(string)
	if !ok {
		b.Fatalf("failed to get GUTI from registration")
	}

	// Create session.
	session, err := initSessionErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
	if err != nil {
		b.Fatalf("failed to initialize session: %v", err)
	}

	// Force GC and get baseline.
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineHeap := memStats.HeapAlloc

	b.Logf("\n=== Transition Memory Growth Tracking ===")
	b.Logf("Baseline heap: %d bytes (%.2f MB)", baselineHeap, float64(baselineHeap)/(1024*1024))

	// Reset timer to exclude setup time.
	b.ResetTimer()

	// Track memory every N operations.
	sampleInterval := 1
	if b.N > 10 {
		sampleInterval = b.N / 10
	}

	// Run benchmark - only measure transition cycles.
	for i := 0; i < b.N; i++ {
		// Transition to idle.
		ctxRel, err := initContextReleaseErr(ctx, name, namespace, guti, sessionId, statusCond{"Ready", "True"})
		if err != nil {
			b.Fatalf("failed to create context release %d: %v", i, err)
		}

		// Transition back to active.
		if err := c.Delete(ctx, ctxRel); err != nil && !apierrors.IsNotFound(err) {
			b.Fatalf("failed to delete context release %d: %v", i, err)
		}

		// Wait for UPF Config to reappear.
		upfConfig := object.NewViewObject("upf", "Config")
		object.SetName(upfConfig, namespace, name)

		ticker := time.NewTicker(interval)
		timeoutTimer := time.NewTimer(timeout)
		configReady := false

	loopGrowth:
		for {
			select {
			case <-timeoutTimer.C:
				ticker.Stop()
				b.Fatalf("timeout waiting for UPF config to reappear for transition %d", i)
			case <-ticker.C:
				if err := c.Get(ctx, client.ObjectKeyFromObject(upfConfig), upfConfig); err == nil {
					configReady = true
					break loopGrowth
				}
			}
		}
		ticker.Stop()
		timeoutTimer.Stop()

		if !configReady {
			b.Fatalf("UPF config did not reappear for transition %d", i)
		}

		// Sample memory at intervals.
		if (i+1)%sampleInterval == 0 {
			runtime.ReadMemStats(&memStats)
			currentHeap := memStats.HeapAlloc
			growth := int64(currentHeap) - int64(baselineHeap)
			perTransition := growth / int64(i+1)
			b.Logf("After %d transitions: heap=%d bytes (%.2f MB), growth=%.2f MB, per-transition=%.2f KB",
				i+1,
				currentHeap,
				float64(currentHeap)/(1024*1024),
				float64(growth)/(1024*1024),
				float64(perTransition)/1024)
		}
	}

	// Stop timer before cleanup.
	b.StopTimer()

	// Final memory check.
	runtime.ReadMemStats(&memStats)
	finalHeap := memStats.HeapAlloc
	totalGrowth := int64(finalHeap) - int64(baselineHeap)

	b.Logf("\n=== Final Transition Memory Report ===")
	b.Logf("Final heap: %d bytes (%.2f MB)", finalHeap, float64(finalHeap)/(1024*1024))
	b.Logf("Total growth: %.2f MB", float64(totalGrowth)/(1024*1024))
	b.Logf("Average per transition: %.2f KB", float64(totalGrowth)/float64(b.N)/1024)
	b.Logf("Note: Measurements exclude reg+session creation time")

	// Cleanup: delete session first, then registration.
	if err := c.Delete(ctx, session); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete session %s/%s: %v",
			session.GetNamespace(), session.GetName(), err)
	}
	if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
		b.Logf("warning: failed to delete registration %s/%s: %v",
			reg.GetNamespace(), reg.GetName(), err)
	}

	// Check memory after cleanup.
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	afterCleanup := memStats.HeapAlloc
	b.Logf("After cleanup: %d bytes (%.2f MB), leaked: %.2f MB",
		afterCleanup,
		float64(afterCleanup)/(1024*1024),
		float64(int64(afterCleanup)-int64(baselineHeap))/(1024*1024))
}
