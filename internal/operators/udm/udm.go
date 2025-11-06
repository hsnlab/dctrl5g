// UDM: Unified Data Management operator
//
// Logical Functions within UDM (internal to UDM):
// - ARPF - Authentication credential Repository and Processing Function (contains subscriber credentials)
// - SIDF - Subscription Identifier De-concealing Function (resolves SUPI from SUCI)
package udm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	"github.com/l7mp/dcontroller/pkg/manager"
	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"
	"github.com/l7mp/dcontroller/pkg/predicate"
	"github.com/l7mp/dcontroller/pkg/reconciler"
)

const (
	OperatorName = "udm"
	// RBACRules = `[{"verbs":["*"],"apiGroups":["*"],"resources":[]}]`
)

type UDM struct {
	*operator.Operator
	c *udmController
}

func New(keyFile string, apiServer *apiserver.APIServer, logger logr.Logger) (*UDM, error) {
	log := logger.WithName("udm")

	mgr, err := manager.New(ctrl.GetConfigOrDie(), OperatorName, manager.Options{})
	if err != nil {
		return nil, err
	}

	// Load the operator from file
	errorChan := make(chan error, 16)
	opts := operator.Options{
		APIServer:    apiServer,
		ErrorChannel: errorChan,
		Logger:       logger,
	}

	op := operator.New(OperatorName, mgr, &opv1a1.OperatorSpec{}, opts)

	// Create the udm controller
	c, err := NewUdmController(mgr, keyFile, apiServer.GetServerAddress(), logger)
	if err != nil {
		return nil, err
	}

	log.Info("created udm controller")

	return &UDM{Operator: op, c: c}, nil
}

func (u *UDM) GetGVKs() []schema.GroupVersionKind {
	return u.c.gvks
}

// udmController implements the udm controller
type udmController struct {
	client.Client
	keyFile       string
	serverAddress string
	generator     *auth.TokenGenerator
	gvks          []schema.GroupVersionKind
	log           logr.Logger
}

func NewUdmController(mgr *manager.Manager, keyFile, serverAddress string, log logr.Logger) (*udmController, error) {
	privateKey, err := auth.LoadPrivateKey(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key %q: %w", keyFile, err)
	}
	generator := auth.NewTokenGenerator(privateKey)

	r := &udmController{
		Client:        mgr.GetClient(),
		keyFile:       keyFile,
		generator:     generator,
		serverAddress: serverAddress,
		log:           log.WithName("udm-ctrl"),
	}

	on := true
	c, err := controller.NewTyped("udm-controller", mgr, controller.TypedOptions[reconciler.Request]{
		SkipNameValidation: &on,
		Reconciler:         r,
	})
	if err != nil {
		return nil, err
	}

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
		Insecure:         true,
		HTTPMode:         false,
	}

	config := auth.GenerateKubeconfig(r.serverAddress, guti, token, kubeconfigOpts)

	// convert to unstructured
	jsonData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	kubeconfig := map[string]any{}
	if err := json.Unmarshal(jsonData, &kubeconfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from JSON: %w", err)
	}

	return kubeconfig, nil

}

func (r *udmController) setStatus(ctx context.Context, obj object.Object, result, reason, message string, config map[string]any) {
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
