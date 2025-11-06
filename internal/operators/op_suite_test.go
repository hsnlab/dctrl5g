package operators

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	timeout       = time.Second * 5
	interval      = time.Millisecond * 50
	retryInterval = time.Millisecond * 100
)

var (
	loglevel = -10
	logger   = zap.New(zap.UseFlagOptions(&zap.Options{
		Development:     true,
		DestWriter:      GinkgoWriter,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
		Level:           zapcore.Level(loglevel),
	}))
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "5G Operators")
}

// func tryWatch(watcher watch.Interface, d time.Duration) (watch.Event, bool) {
// 	select {
// 	case event := <-watcher.ResultChan():
// 		return event, true
// 	case <-time.After(d):
// 		return watch.Event{}, false
// 	}
// }
