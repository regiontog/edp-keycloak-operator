package keycloakrealm

import (
	"context"
	coreerrors "errors"
	"gopkg.in/nerzal/gocloak.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"keycloak-operator/pkg/adapter/keycloak"
	v1v1alpha1 "keycloak-operator/pkg/apis/v1/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_keycloakrealm")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KeycloakRealm Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	adapter := keycloak.GoCloakAdapter{
		ClientSup: func(url string) gocloak.GoCloak {
			return gocloak.NewClient(url)
		},
	}
	return &ReconcileKeycloakRealm{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		adapter: adapter}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("keycloakrealm-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeycloakRealm
	return c.Watch(&source.Kind{Type: &v1v1alpha1.KeycloakRealm{}}, &handler.EnqueueRequestForObject{})
}

// blank assignment to verify that ReconcileKeycloakRealm implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeycloakRealm{}

// ReconcileKeycloakRealm reconciles a KeycloakRealm object
type ReconcileKeycloakRealm struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client  client.Client
	scheme  *runtime.Scheme
	adapter keycloak.IGoCloakAdapter
}

// Reconcile reads that state of the cluster for a KeycloakRealm object and makes changes based on the state read
// and what is in the KeycloakRealm.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeycloakRealm) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeycloakRealm")

	// Fetch the KeycloakRealm instance
	instance := &v1v1alpha1.KeycloakRealm{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	ownerKeycloak, err := r.getOwnerKeycloak(instance)
	if err != nil {
		return reconcile.Result{}, nil
	}

	if ownerKeycloak.Status.Connected {
		err = r.putRealm(ownerKeycloak, instance)
	}

	return reconcile.Result{}, err
}

func (r *ReconcileKeycloakRealm) putRealm(owner *v1v1alpha1.Keycloak, realm *v1v1alpha1.KeycloakRealm) error {
	reqLog := log.WithValues("keycloak cr", owner, "realm cr", realm)
	reqLog.Info("Start putting realm")
	connection, err := r.adapter.GetConnection(*owner)
	if err != nil {
		return err
	}
	realmRepresentation, err := connection.Client.GetRealm(connection.Token.AccessToken, realm.Spec.RealmName)
	if err != nil {
		reqLog.Error(err, "error by the get realm request")
	}
	if realmRepresentation == nil {
		err = connection.Client.CreateRealm(connection.Token.AccessToken, gocloak.RealmRepresentation{
			Realm: realm.Spec.RealmName,
		})
	}
	reqLog.Info("End putting realm")
	return nil
}

func (r *ReconcileKeycloakRealm) getOwnerKeycloak(realm *v1v1alpha1.KeycloakRealm) (*v1v1alpha1.Keycloak, error) {
	reqLog := log.WithValues("realm cr", realm)
	reqLog.Info("Start getting owner Keycloak")

	ows := realm.GetOwnerReferences()
	if len(ows) == 0 {
		return nil, coreerrors.New("keycloak realm cr does not have owner references")
	}
	keycloakOwner := getKeycloakOwner(ows)
	if keycloakOwner == nil {
		return nil, coreerrors.New("keycloak realm cr does not keycloak cr owner references")
	}

	nsn := types.NamespacedName{
		Namespace: realm.Namespace,
		Name:      keycloakOwner.Name,
	}

	ownerCr := &v1v1alpha1.Keycloak{}
	err := r.client.Get(context.TODO(), nsn, ownerCr)
	return ownerCr, err
}

func getKeycloakOwner(references []v1.OwnerReference) *v1.OwnerReference {
	for _, el := range references {
		if el.Kind == "Keycloak" {
			return &el
		}
	}
	return nil
}