package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/hsnlab/dctrl5g/internal/buildinfo"
	"github.com/hsnlab/dctrl5g/internal/dctrl"
)

const (
	APIServerPort = 8443
	CtrlDir       = "operators/"
)

var (
	scheme     = runtime.NewScheme()
	version    = "dev"
	commitHash = "n/a"
	buildDate  = "<unknown>"
)

func main() {
	opts := zap.Options{
		Development:     true,
		DestWriter:      os.Stderr,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger.WithName("dctrl5g"))
	setupLog := logger.WithName("setup")

	buildInfo := buildinfo.BuildInfo{Version: version, CommitHash: commitHash, BuildDate: buildDate}
	setupLog.Info(fmt.Sprintf("starting the dctrl5g %s", buildInfo.String()))

	dctrl, err := dctrl.New(dctrl.Options{
		CtrlDir:       CtrlDir,
		APIServerPort: APIServerPort,
		Logger:        logger,
	})
	if err != nil {
		setupLog.Error(err, "failed to init")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	dctrl.Start(ctx)
}
