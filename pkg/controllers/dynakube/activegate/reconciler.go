package activegate

import (
	dynatracev1beta2 "github.com/Dynatrace/dynatrace-operator/pkg/api/v1beta2/dynakube"
	dtclient "github.com/Dynatrace/dynatrace-operator/pkg/clients/dynatrace"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/activegate/capability"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/activegate/internal/authtoken"
	capabilityInternal "github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/activegate/internal/capability"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/activegate/internal/customproperties"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/activegate/internal/statefulset"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/connectioninfo"
	agconnectioninfo "github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/connectioninfo/activegate"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/dtpullsecret"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/istio"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/token"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/version"
	"github.com/Dynatrace/dynatrace-operator/pkg/util/kubeobjects/configmap"
	"github.com/Dynatrace/dynatrace-operator/pkg/util/kubeobjects/object"
	"github.com/Dynatrace/dynatrace-operator/pkg/util/timeprovider"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	client                            client.Client
	dynakube                          *dynatracev1beta2.DynaKube
	apiReader                         client.Reader
	authTokenReconciler               controllers.Reconciler
	istioReconciler                   istio.Reconciler
	connectionReconciler              controllers.Reconciler
	versionReconciler                 version.Reconciler
	pullSecretReconciler              controllers.Reconciler
	newStatefulsetReconcilerFunc      statefulset.NewReconcilerFunc
	newCapabilityReconcilerFunc       capabilityInternal.NewReconcilerFunc
	newCustomPropertiesReconcilerFunc func(customPropertiesOwnerName string, customPropertiesSource *dynatracev1beta2.DynaKubeValueSource) controllers.Reconciler
}

var _ controllers.Reconciler = (*Reconciler)(nil)

type ReconcilerBuilder func(clt client.Client,
	apiReader client.Reader,
	dynakube *dynatracev1beta2.DynaKube,
	dtc dtclient.Client,
	istioClient *istio.Client,
	tokens token.Tokens,
) controllers.Reconciler

func NewReconciler(clt client.Client, //nolint
	apiReader client.Reader,
	dynakube *dynatracev1beta2.DynaKube,
	dtc dtclient.Client,
	istioClient *istio.Client,
	tokens token.Tokens) controllers.Reconciler {
	var istioReconciler istio.Reconciler
	if istioClient != nil {
		istioReconciler = istio.NewReconciler(istioClient)
	}

	authTokenReconciler := authtoken.NewReconciler(clt, apiReader, dynakube, dtc)
	versionReconciler := version.NewReconciler(apiReader, dtc, timeprovider.New().Freeze())
	connectionInfoReconciler := agconnectioninfo.NewReconciler(clt, apiReader, dtc, dynakube)
	pullSecretReconciler := dtpullsecret.NewReconciler(clt, apiReader, dynakube, tokens)

	newCustomPropertiesReconcilerFunc := func(customPropertiesOwnerName string, customPropertiesSource *dynatracev1beta2.DynaKubeValueSource) controllers.Reconciler {
		return customproperties.NewReconciler(clt, dynakube, customPropertiesOwnerName, customPropertiesSource)
	}

	return &Reconciler{
		client:                            clt,
		apiReader:                         apiReader,
		dynakube:                          dynakube,
		authTokenReconciler:               authTokenReconciler,
		istioReconciler:                   istioReconciler,
		connectionReconciler:              connectionInfoReconciler,
		versionReconciler:                 versionReconciler,
		pullSecretReconciler:              pullSecretReconciler,
		newCustomPropertiesReconcilerFunc: newCustomPropertiesReconcilerFunc,
		newStatefulsetReconcilerFunc:      statefulset.NewReconciler,
		newCapabilityReconcilerFunc:       capabilityInternal.NewReconciler,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	err := r.createActiveGateTenantConnectionInfoConfigMap(ctx)
	if err != nil {
		return err
	}

	err = r.connectionReconciler.Reconcile(ctx)
	if err != nil {
		return err
	}

	err = r.versionReconciler.ReconcileActiveGate(ctx, r.dynakube)
	if err != nil {
		return err
	}

	err = r.pullSecretReconciler.Reconcile(ctx)
	if err != nil {
		return err
	}

	if r.istioReconciler != nil {
		err = r.istioReconciler.ReconcileActiveGateCommunicationHosts(ctx, r.dynakube)
		if err != nil {
			return err
		}
	}

	err = r.authTokenReconciler.Reconcile(ctx)
	if err != nil {
		return errors.WithMessage(err, "could not reconcile Dynatrace ActiveGateAuthToken secrets")
	}

	for _, agCapability := range capability.GenerateActiveGateCapabilities(r.dynakube) {
		if agCapability.Enabled() {
			return r.createCapability(ctx, agCapability)
		} else {
			if err := r.deleteCapability(ctx, agCapability); err != nil {
				return err
			}
		}
	}

	// TODO: move cleanup to ActiveGate reconciler
	meta.RemoveStatusCondition(r.dynakube.Conditions(), statefulset.ActiveGateStatefulSetConditionType)

	return nil
}

func (r *Reconciler) createActiveGateTenantConnectionInfoConfigMap(ctx context.Context) error {
	if !r.dynakube.NeedsActiveGate() {
		// TODO: Add clean up of the config map
		return nil
	}

	configMapData := extractPublicData(r.dynakube)

	configMap, err := configmap.CreateConfigMap(r.dynakube,
		configmap.NewModifier(r.dynakube.ActiveGateConnectionInfoConfigMapName()),
		configmap.NewNamespaceModifier(r.dynakube.Namespace),
		configmap.NewConfigMapDataModifier(configMapData))
	if err != nil {
		return errors.WithStack(err)
	}

	query := configmap.NewQuery(ctx, r.client, r.apiReader, log)

	err = query.CreateOrUpdate(*configMap)
	if err != nil {
		log.Info("could not create or update configMap for connection info", "name", configMap.Name)

		return err
	}

	return nil
}

func extractPublicData(dynakube *dynatracev1beta2.DynaKube) map[string]string {
	data := map[string]string{}

	if dynakube.Status.ActiveGate.ConnectionInfoStatus.TenantUUID != "" {
		data[connectioninfo.TenantUUIDKey] = dynakube.Status.ActiveGate.ConnectionInfoStatus.TenantUUID
	}

	if dynakube.Status.ActiveGate.ConnectionInfoStatus.Endpoints != "" {
		data[connectioninfo.CommunicationEndpointsKey] = dynakube.Status.ActiveGate.ConnectionInfoStatus.Endpoints
	}

	return data
}

func (r *Reconciler) createCapability(ctx context.Context, agCapability capability.Capability) error {
	customPropertiesReconciler := r.newCustomPropertiesReconcilerFunc(r.dynakube.ActiveGateServiceAccountOwner(), agCapability.Properties().CustomProperties) //nolint:typeCheck
	statefulsetReconciler := r.newStatefulsetReconcilerFunc(r.client, r.apiReader, r.dynakube, agCapability)                                                  //nolint:typeCheck

	capabilityReconciler := r.newCapabilityReconcilerFunc(r.client, agCapability, r.dynakube, statefulsetReconciler, customPropertiesReconciler)

	return capabilityReconciler.Reconcile(ctx)
}

func (r *Reconciler) deleteCapability(ctx context.Context, agCapability capability.Capability) error {
	if err := r.deleteStatefulset(ctx, agCapability); err != nil {
		return err
	}

	if err := r.deleteService(ctx, agCapability); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) deleteService(ctx context.Context, agCapability capability.Capability) error {
	if r.dynakube.NeedsActiveGateService() {
		return nil
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      capability.BuildServiceName(r.dynakube.Name, agCapability.ShortName()),
			Namespace: r.dynakube.Namespace,
		},
	}

	return object.Delete(ctx, r.client, &svc)
}

func (r *Reconciler) deleteStatefulset(ctx context.Context, agCapability capability.Capability) error {
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      capability.CalculateStatefulSetName(agCapability, r.dynakube.Name),
			Namespace: r.dynakube.Namespace,
		},
	}

	return object.Delete(ctx, r.client, &sts)
}
