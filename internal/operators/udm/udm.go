// UDM: Unified Data Management operator
//
// Logical Functions within UDM (internal to UDM):
// - ARPF - Authentication credential Repository and Processing Function (contains subscriber credentials)
// - SIDF - Subscription Identifier De-concealing Function (resolves SUPI from SUCI)
package udm

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	runtimeMgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	dcontroller "github.com/l7mp/dcontroller/pkg/controller"
	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"
	"github.com/l7mp/dcontroller/pkg/predicate"
	"github.com/l7mp/dcontroller/pkg/reconciler"
)

const (
	OperatorName = "udm"
	// RBACRules = `[{"verbs":["*"],"apiGroups":["*"],"resources":[]}]`
)

type Options struct {
	HTTPMode, Insecure bool
	KeyFile            string
	Logger             logr.Logger
}

type UDM struct {
	*operator.Operator
	c *udmController
}

func New(mgr runtimeMgr.Manager, apiServer *apiserver.APIServer, opts Options) (*UDM, error) {
	log := opts.Logger.WithName("udm")

	// Load the operator from file
	errorChan := make(chan error, 16)
	op := operator.New(OperatorName, mgr, operator.Options{
		APIServer:    apiServer,
		ErrorChannel: errorChan,
		Logger:       opts.Logger,
	})

	// Create the udm controller
	c, err := NewUdmController(mgr, apiServer.GetServerAddress(), opts)
	if err != nil {
		return nil, err
	}

	log.Info("created udm controller")

	// Add native controller to the operator and export GVKs to the API server.
	op.AddNativeController("config-ctrl", c.ctrl, c.gvks)

	return &UDM{Operator: op, c: c}, nil
}

func (u *UDM) GetGVKs() []schema.GroupVersionKind { return u.c.gvks }

// udmController implements the udm controller
type udmController struct {
	client.Client
	opts          Options
	serverAddress string
	generator     *auth.TokenGenerator
	ctrl          dcontroller.RuntimeController
	gvks          []schema.GroupVersionKind
	log           logr.Logger
}

func NewUdmController(mgr runtimeMgr.Manager, serverAddress string, opts Options) (*udmController, error) {
	privateKey, err := auth.LoadPrivateKey(opts.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key %q: %w", opts.KeyFile, err)
	}
	generator := auth.NewTokenGenerator(privateKey)

	r := &udmController{
		Client:        mgr.GetClient(),
		opts:          opts,
		generator:     generator,
		serverAddress: serverAddress,
		gvks:          []schema.GroupVersionKind{},
		log:           opts.Logger.WithName("udm-ctrl"),
	}

	on := true
	c, err := controller.NewTyped("udm-controller", mgr, controller.TypedOptions[reconciler.Request]{
		SkipNameValidation: &on,
		Reconciler:         r,
	})
	if err != nil {
		return nil, err
	}
	r.ctrl = c

	p := predicate.BasicPredicate("GenerationChanged")
	s := reconciler.NewSource(mgr, OperatorName, opv1a1.Source{
		Resource: opv1a1.Resource{
			Kind: "Config",
		},
		Predicate: &predicate.Predicate{BasicPredicate: &p},
	})
	gvk, err := s.GetGVK()
	if err != nil {
		return nil, fmt.Errorf("failed to get GVK for source: %w", err)
	}
	r.gvks = append(r.gvks, gvk)

	src, err := s.GetSource()
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	if err := c.Watch(src); err != nil {
		return nil, fmt.Errorf("failed to create watch: %w", err)
	}

	r.log.Info("created UDM controller")

	return r, nil
}

func (r *udmController) Reconcile(ctx context.Context, req reconciler.Request) (reconcile.Result, error) {
	r.log.Info("Reconciling", "request", req.String())

	switch req.EventType {
	case object.Added, object.Updated, object.Upserted:
		obj := object.NewViewObject(OperatorName, req.GVK.Kind)
		if err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, obj); err != nil {
			r.log.Error(err, "failed to get added/updated object", "delta-type", req.EventType)
			return reconcile.Result{}, err
		}

		name := obj.GetName()
		namespace := obj.GetNamespace()

		r.log.Info("Add/update Config request object", "name", name, "namespace", namespace)

		config, err := r.getKubeConfig(obj)
		if err != nil {
			r.setStatus(ctx, obj, "False", "ConfigUnavailable", "Failed to generate config", nil)
			return reconcile.Result{},
				fmt.Errorf("failed to generate config: %w", err)
		}

		r.setStatus(ctx, obj, "True", "Ready", "Succesfully generated config", config)

	case object.Deleted:
		r.log.Info("Delete Config object", "name", req.Name, "namespace", req.Namespace)

		// do nothing

	default:
		r.log.Info("Unhandled event", "name", req.Name, "namespace", req.Namespace, "type", req.EventType)
	}

	r.log.Info("Reconciliation done")

	return reconcile.Result{}, nil
}

func (r *udmController) getKubeConfig(obj object.Object) (map[string]any, error) {
	guti := obj.GetName()
	namespacesList := []string{guti}
	rulesList := []rbacv1.PolicyRule{}
	token, err := r.generator.GenerateToken(guti, namespacesList, rulesList, 168*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Create kubeconfig
	kubeconfigOpts := &auth.KubeconfigOptions{
		ClusterName:      "dctrl5g",
		ContextName:      "dctrl5g",
		DefaultNamespace: "default",
		Insecure:         r.opts.Insecure,
		HTTPMode:         r.opts.HTTPMode,
	}

	config := auth.GenerateKubeconfig(r.serverAddress, guti, token, kubeconfigOpts)

	// convert to unstructured
	yamlData, err := clientcmd.Write(*config)
	if err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig YAML: %w", err)
	}

	kubeconfig := map[string]any{}
	if err := yaml.Unmarshal(yamlData, &kubeconfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from JSON: %w", err)
	}

	return kubeconfig, nil

}

func (r *udmController) setStatus(ctx context.Context, obj object.Object, result, reason, message string, config map[string]any) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["state"] = "ConfigAvailable"
	obj.SetLabels(labels)

	condition := map[string]any{
		"lastTransitionTime": time.Now().String(),
		"type":               "Ready",
		"status":             result,
		"reason":             reason,
		"message":            message,
	}

	status := map[string]any{"conditions": []any{condition}}
	if config != nil {
		status["config"] = config
	}

	if err := unstructured.SetNestedMap(obj.UnstructuredContent(), status, "status"); err != nil {
		r.log.Error(err, "failed to set config status")
	}

	if err := r.Update(ctx, obj); err != nil {
		r.log.Error(err, "failed to update object", "key", client.ObjectKeyFromObject(obj))
	}
}
