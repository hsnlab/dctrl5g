package operators

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	dmanager "github.com/l7mp/dcontroller/pkg/manager"
	"github.com/l7mp/dcontroller/pkg/operator"
)

const (
	timeout       = time.Second * 1
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

func startOp(ctx context.Context, opFileName string) (*operator.Operator, error) {
	specFile, err := os.Open(opFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open spec file for operator spec file %q: %w",
			opFileName, err)
	}
	defer specFile.Close() //nolint:errcheck

	data, err := io.ReadAll(specFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file for operator spec file %q: %w",
			opFileName, err)
	}

	opSpec := &opv1a1.Operator{}
	if err := yaml.Unmarshal(data, &opSpec); err != nil {
		return nil, fmt.Errorf("failed to parse operator spec file %q: %w", opFileName, err)
	}

	mgr, err := dmanager.New(nil, opSpec.GetName(), dmanager.Options{Options: manager.Options{
		Metrics: metrics.Options{
			BindAddress: ":54322",
		},
		HealthProbeBindAddress: "",
		Logger:                 logger,
	}})
	if err != nil {
		return nil, err
	}

	errorChan := make(chan error, 16)
	op := operator.New(opSpec.GetName(), mgr, &opSpec.Spec, operator.Options{
		ErrorChannel: errorChan,
		Logger:       logger,
	})

	go func() {
		defer GinkgoRecover()
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errorChan:
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}()

	go func() {
		defer GinkgoRecover()
		err := op.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	return op, nil
}

// func tryWatch(watcher watch.Interface, d time.Duration) (watch.Event, bool) {
// 	select {
// 	case event := <-watcher.ResultChan():
// 		return event, true
// 	case <-time.After(d):
// 		return watch.Event{}, false
// 	}
// }
