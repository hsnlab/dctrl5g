package dctrl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/controller"
	"github.com/l7mp/dcontroller/pkg/operator"
)

const (
	DefaultCtrlDir = "controllers/"
)

type OpSpec struct {
	Name, Spec string
}

type Options struct {
	CtrlDir       string
	APIServerPort int
	Logger        logr.Logger
}

type dctrl struct {
	*operator.Group
	apiServer   *apiserver.APIServer
	log, logger logr.Logger
}

func New(opts Options) (*dctrl, error) {
	logger := opts.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}

	ctrlDir := opts.CtrlDir
	if ctrlDir == "" {
		ctrlDir = DefaultCtrlDir
	}

	config := &rest.Config{
		Host: fmt.Sprintf("http://0.0.0.0:%d", opts.APIServerPort),
	}

	// 1. Create an operator group that will run our operators
	g := operator.NewGroup(config, logger)

	// 2. Create an API server: this will multiplex all operator-to-operator comms
	apiServerConfig, err := apiserver.NewDefaultConfig("0.0.0.0", opts.APIServerPort,
		g.GetClient(), true, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create the config for the embedded API server: %w", err)
	}
	apiServer, err := apiserver.NewAPIServer(apiServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create the embedded API server: %w", err)
	}

	g.SetAPIServer(apiServer)

	// 3. Create the operators
	ctrlFiles, err := os.ReadDir(ctrlDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open controller directory %q: %w", ctrlDir, err)
	}

	for _, ctrlFile := range ctrlFiles {
		if ctrlFile.IsDir() {
			continue
		}

		filePath := filepath.Join(ctrlDir, ctrlFile.Name())

		specFile, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open spec file for controller spec file %q: %w",
				ctrlFile.Name(), err)
		}
		defer specFile.Close()

		data, err := io.ReadAll(specFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read spec file for controller spec file %q: %w",
				ctrlFile.Name(), err)
		}

		name, spec, init, err := parseRawSpec(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse operator spec file %q: %w",
				ctrlFile.Name(), err)
		}

		op, err := g.AddOperator(name, spec)
		if err != nil {
			return nil, fmt.Errorf("unable to create operator %q: %w", name, err)
		}

		if err := initOp(op, init); err != nil {
			return nil, fmt.Errorf("unable to init operator %q: %w", name, err)
		}
	}

	return &dctrl{Group: g, apiServer: apiServer, log: logger.WithName("dctrl"), logger: logger}, nil
}

func (d *dctrl) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		d.log.V(1).Info("starting API server")
		if err := d.apiServer.Start(ctx); err != nil {
			d.log.Error(err, "embedded API server error")
			cancel()
		}
	}()

	go func() {
		d.log.V(1).Info("starting the operator group")
		if err := d.Group.Start(ctx); err != nil {
			d.log.Error(err, "operator group error")
			cancel()
		}
	}()

	for {
		select {
		case err := <-d.GetErrorChannel():
			var operr controller.Error
			if errors.As(err, &operr) {
				d.log.Error(err, "controller error", "operator", operr.Operator,
					"controller", operr.Controller)
			} else {
				d.log.Error(err, "unknown error")
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func parseRawSpec(data []byte) (string, *opv1a1.OperatorSpec, map[string]any, error) {
	opSpec := map[string]any{}
	if err := yaml.Unmarshal(data, &opSpec); err != nil {
		return "", nil, nil, fmt.Errorf("failed to parse YAML spec: %w", err)
	}

	rawName, ok := opSpec["name"]
	if !ok {
		return "", nil, nil, errors.New("no operator name")
	}

	name, ok := rawName.(string)
	if !ok {
		return "", nil, nil, errors.New("expected string for operator name")
	}

	rawSpec, ok := opSpec["spec"]
	if !ok {
		return "", nil, nil, errors.New("no operator spec")
	}

	spec, ok := rawSpec.(*opv1a1.OperatorSpec)
	if !ok {
		return "", nil, nil, errors.New("expected operator spec")
	}

	var init map[string]any
	if rawInit, ok := opSpec["init"]; ok {
		init, ok = rawInit.(map[string]any)
	}

	return name, spec, init, nil
}

func initOp(op *operator.Operator, init map[string]any) error {
	return nil
}
