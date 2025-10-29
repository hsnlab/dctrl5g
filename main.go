package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap/zapcore"
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
	flags := flag.NewFlagSet("dctrl5g", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of dctrl5g:\n")
		flags.PrintDefaults()
	}
	addr := flags.String("addr", "localhost", "API server bind address")
	port := flags.Int("port", 8443, "API server port")
	httpMode := flags.Bool("http", false, "Use insecure HTTP (no TLS)")
	certFile := flags.String("tls-cert-file", "apiserver.crt",
		"TLS cert file for secure mode and JWT validation (latter not required if --disable-authentication is set)")
	keyFile := flags.String("tls-key-file", "apiserver.key", "TLS key file for secure mode")
	disableAuthentication := flags.Bool("disable-authentication", false,
		"Disable authentication/authorization (WARNING: allows unrestricted access)")
	opts.BindFlags(flags)
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		flags.Usage()
		os.Exit(2)
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger.WithName("dctrl5g"))
	setupLog := logger.WithName("setup")

	buildInfo := buildinfo.BuildInfo{Version: version, CommitHash: commitHash, BuildDate: buildDate}
	setupLog.Info(fmt.Sprintf("starting the dctrl5g %s", buildInfo.String()))

	dctrl, err := dctrl.New(dctrl.Options{
		CtrlDir:       CtrlDir,
		APIServerAddr: *addr,
		APIServerPort: *port,
		HTTPMode:      *httpMode,
		DisableAuth:   *disableAuthentication,
		CertFile:      *certFile,
		KeyFile:       *keyFile,
		Logger:        logger,
	})
	if err != nil {
		setupLog.Error(err, "failed to init")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	if err := dctrl.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}
}
