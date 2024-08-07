package injection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Dynatrace/dynatrace-operator/pkg/api/scheme/fake"
	dynatracev1beta2 "github.com/Dynatrace/dynatrace-operator/pkg/api/v1beta2/dynakube"
	dtclient "github.com/Dynatrace/dynatrace-operator/pkg/clients/dynatrace"
	"github.com/Dynatrace/dynatrace-operator/pkg/consts"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers"
	"github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/istio"
	versions "github.com/Dynatrace/dynatrace-operator/pkg/controllers/dynakube/version"
	"github.com/Dynatrace/dynatrace-operator/pkg/injection/namespace/mapper"
	"github.com/Dynatrace/dynatrace-operator/pkg/injection/startup"
	dtwebhook "github.com/Dynatrace/dynatrace-operator/pkg/webhook"
	dtclientmock "github.com/Dynatrace/dynatrace-operator/test/mocks/pkg/clients/dynatrace"
	controllermock "github.com/Dynatrace/dynatrace-operator/test/mocks/pkg/controllers"
	versionmock "github.com/Dynatrace/dynatrace-operator/test/mocks/pkg/controllers/dynakube/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	istio2 "istio.io/api/networking/v1beta1"
	istiov1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	fakeistio "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	testPaasToken       = "test-paas-token"
	testAPIToken        = "test-api-token"
	testDataIngestToken = "test-ingest-token"

	testUUID                  = "test-uuid"
	testTenantToken           = "abcd"
	testCommunicationEndpoint = "https://tenant.dev.dynatracelabs.com:443"

	testHost = "test-host"

	testDynakube   = "test-name"
	testDynakube2  = "test-name2"
	testNamespace  = "test-namespace"
	testNamespace2 = "test-namespace2"

	testNamespaceSelectorLabel = "namespaceSelector"

	testNamespaceDynatrace = "dynatrace"

	testApiUrl = "https://" + testHost + "/e/" + testUUID + "/api"
)

func TestReconciler(t *testing.T) {
	t.Run("add injection", func(t *testing.T) {
		expectedOneAgentConnectionInfo := dtclient.OneAgentConnectionInfo{
			ConnectionInfo: dtclient.ConnectionInfo{
				TenantUUID:  testUUID,
				TenantToken: testTenantToken,
				Endpoints:   testCommunicationEndpoint,
			},
			CommunicationHosts: []dtclient.CommunicationHost{
				{
					Protocol: "https",
					Host:     "tenant.dev.dynatracelabs.com",
					Port:     443,
				},
				{
					Protocol: "https",
					Host:     "1.2.3.4",
					Port:     443,
				},
			},
		}
		dynakube := &dynatracev1beta2.DynaKube{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDynakube,
				Namespace: testNamespaceDynatrace,
			},
			Spec: dynatracev1beta2.DynaKubeSpec{
				APIURL: testApiUrl,
				OneAgent: dynatracev1beta2.OneAgentSpec{
					CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{
						AppInjectionSpec: dynatracev1beta2.AppInjectionSpec{
							NamespaceSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									testNamespaceSelectorLabel: testDynakube,
								},
							},
						},
					},
				},
				MetadataEnrichment: dynatracev1beta2.MetadataEnrichment{
					Enabled: true,
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							testNamespaceSelectorLabel: testDynakube,
						},
					},
				},
			},
		}
		clt := fake.NewClientWithIndex(
			clientNotInjectedNamespace(testNamespace, testDynakube),
			clientNotInjectedNamespace(testNamespace2, testDynakube2),
			clientSecret(testDynakube, testNamespaceDynatrace, map[string][]byte{
				dtclient.ApiToken:  []byte(testAPIToken),
				dtclient.PaasToken: []byte(testPaasToken),
			}),
			dynakube,
		)
		dtClient := dtclientmock.NewClient(t)
		dtClient.On("GetLatestAgentVersion", mock.AnythingOfType("context.backgroundCtx"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", nil)
		dtClient.On("GetOneAgentConnectionInfo", mock.AnythingOfType("context.backgroundCtx")).Return(expectedOneAgentConnectionInfo, nil)
		dtClient.On("GetProcessModuleConfig", mock.AnythingOfType("context.backgroundCtx"), mock.AnythingOfType("uint")).Return(&dtclient.ProcessModuleConfig{}, nil)
		dtClient.On("GetRulesSettings", mock.AnythingOfType("context.backgroundCtx"), mock.AnythingOfType("string")).Return(dtclient.GetRulesSettingsResponse{}, nil)

		istioClient := newIstioTestingClient(fakeistio.NewSimpleClientset(), dynakube)

		rec := NewReconciler(clt, clt, dtClient, istioClient, dynakube)
		err := rec.Reconcile(context.Background())
		require.NoError(t, err)

		assertSecretFound(t, clt, dynakube.OneagentTenantSecret(), dynakube.Namespace)
		assertSecretFound(t, clt, consts.AgentInitSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace2)
		assertSecretFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace2)

		_, err = istioClient.GetServiceEntry(context.Background(), istio.BuildNameForIPServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		_, err = istioClient.GetServiceEntry(context.Background(), istio.BuildNameForFQDNServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)

		_, err = istioClient.GetVirtualService(context.Background(), istio.BuildNameForIPServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		_, err = istioClient.GetVirtualService(context.Background(), istio.BuildNameForFQDNServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
	})
	t.Run("remove injection", func(t *testing.T) {
		dynakube := &dynatracev1beta2.DynaKube{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDynakube,
				Namespace: testNamespaceDynatrace,
			},
			Spec: dynatracev1beta2.DynaKubeSpec{
				APIURL:      testApiUrl,
				EnableIstio: true,
			},
		}
		setMetadataEnrichmentCreatedCondition(dynakube.Conditions())
		setCodeModulesInjectionCreatedCondition(dynakube.Conditions())

		clt := fake.NewClientWithIndex(
			clientInjectedNamespace(testNamespace, testDynakube),
			clientInjectedNamespace(testNamespace2, testDynakube2),
			clientSecret(consts.EnrichmentEndpointSecretName, testNamespace, nil),
			clientSecret(consts.EnrichmentEndpointSecretName, testNamespace2, nil),
			clientSecret(consts.AgentInitSecretName, testNamespace, nil),
			clientSecret(consts.AgentInitSecretName, testNamespace2, nil),
			clientSecret(testDynakube, testNamespaceDynatrace, map[string][]byte{
				dtclient.ApiToken:  []byte(testAPIToken),
				dtclient.PaasToken: []byte(testPaasToken),
			}),
			dynakube,
		)
		dtClient := dtclientmock.NewClient(t)
		istioClient := setupIstioClientWithObjects(dynakube)

		rec := NewReconciler(clt, clt, dtClient, istioClient, dynakube)
		err := rec.Reconcile(context.Background())
		require.NoError(t, err)

		assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace)
		assertSecretFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace2)
		assert.Nil(t, meta.FindStatusCondition(*dynakube.Conditions(), metaDataEnrichmentConditionType))

		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace)
		assertSecretFound(t, clt, consts.AgentInitSecretName, testNamespace2)
		assert.Nil(t, meta.FindStatusCondition(*dynakube.Conditions(), codeModulesInjectionConditionType))

		obj, err := istioClient.GetServiceEntry(context.Background(), istio.BuildNameForIPServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.Nil(t, obj)
		obj, err = istioClient.GetServiceEntry(context.Background(), istio.BuildNameForFQDNServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.Nil(t, obj)

		virtualService, err := istioClient.GetVirtualService(context.Background(), istio.BuildNameForFQDNServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.Nil(t, virtualService)

		istioClient.Owner.SetNamespace(testNamespace2)
		obj, err = istioClient.GetServiceEntry(context.Background(), istio.BuildNameForIPServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.NotNil(t, obj)
		obj, err = istioClient.GetServiceEntry(context.Background(), istio.BuildNameForIPServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.NotNil(t, obj)

		virtualService, err = istioClient.GetVirtualService(context.Background(), istio.BuildNameForFQDNServiceEntry(dynakube.GetName(), istio.OneAgentComponent))
		require.NoError(t, err)
		assert.NotNil(t, virtualService)
	})
	t.Run(`failure is logged in condition`, func(t *testing.T) {
		dynakube := &dynatracev1beta2.DynaKube{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDynakube,
				Namespace: testNamespaceDynatrace,
			},
			Spec: dynatracev1beta2.DynaKubeSpec{
				APIURL: testApiUrl,
				OneAgent: dynatracev1beta2.OneAgentSpec{
					CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{
						AppInjectionSpec: dynatracev1beta2.AppInjectionSpec{
							NamespaceSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									testNamespaceSelectorLabel: testDynakube,
								},
							},
						},
					},
				},
			},
		}
		boomClient := fake.NewClientWithInterceptors(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return k8serrors.NewInternalError(errors.New("test-error"))
			},
		})

		istioClient := newIstioTestingClient(fakeistio.NewSimpleClientset(), dynakube)
		fakeReconciler := createGenericReconcilerMock(t)
		fakeVersionReconciler := createVersionReconcilerMock(t)

		rec := NewReconciler(boomClient, boomClient, nil, istioClient, dynakube).(*reconciler)
		rec.connectionInfoReconciler = fakeReconciler
		rec.pmcSecretreconciler = fakeReconciler
		rec.versionReconciler = fakeVersionReconciler

		err := rec.Reconcile(context.Background())
		require.Error(t, err)

		condition := meta.FindStatusCondition(*dynakube.Conditions(), codeModulesInjectionConditionType)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
	})
}

func TestRemoveAppInjection(t *testing.T) {
	clt := clientRemoveAppInjection()
	rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
		CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{},
	})
	setCodeModulesInjectionCreatedCondition(rec.dynakube.Conditions())
	setMetadataEnrichmentCreatedCondition(rec.dynakube.Conditions())

	err := rec.removeAppInjection(context.Background())
	require.NoError(t, err)

	var namespace corev1.Namespace
	err = clt.Get(context.Background(), client.ObjectKey{Name: testNamespace, Namespace: ""}, &namespace)
	require.NoError(t, err)
	assert.Nil(t, namespace.ObjectMeta.Labels)
	require.NotNil(t, namespace.ObjectMeta.Annotations)
	assert.Equal(t, "true", namespace.Annotations[mapper.UpdatedViaDynakubeAnnotation])

	err = clt.Get(context.Background(), client.ObjectKey{Name: testNamespace2, Namespace: ""}, &namespace)
	require.NoError(t, err)
	require.NotNil(t, namespace.ObjectMeta.Labels)
	assert.Equal(t, testDynakube2, namespace.Labels[dtwebhook.InjectionInstanceLabel])
	assert.Nil(t, namespace.ObjectMeta.Annotations)

	assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace)
	assertSecretFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace2)
}

func TestSetupOneAgentInjection(t *testing.T) {
	t.Run(`no injection - ClassicFullStack`, func(t *testing.T) {
		clt := clientNoInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			ClassicFullStack: &dynatracev1beta2.HostInjectSpec{},
		})

		err := rec.setupOneAgentInjection(context.Background())
		require.NoError(t, err)

		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace2)
	})

	t.Run(`no injection - HostMonitoring`, func(t *testing.T) {
		clt := clientNoInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			HostMonitoring: &dynatracev1beta2.HostInjectSpec{},
		})

		err := rec.setupOneAgentInjection(context.Background())
		require.NoError(t, err)

		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace2)
	})

	t.Run(`injection - ApplicationMonitoring`, func(t *testing.T) {
		clt := clientOneAgentInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			ApplicationMonitoring: &dynatracev1beta2.ApplicationMonitoringSpec{},
		})

		err := rec.setupOneAgentInjection(context.Background())
		require.NoError(t, err)

		var secret corev1.Secret
		err = clt.Get(context.Background(), client.ObjectKey{Name: consts.AgentInitSecretName, Namespace: testNamespace}, &secret)
		require.NoError(t, err)

		var config startup.SecretConfig
		err = json.Unmarshal(secret.Data["config"], &config)
		require.NoError(t, err)
		assert.Equal(t, testAPIToken, config.ApiToken)
		assert.Equal(t, testPaasToken, config.PaasToken)

		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace2)
	})

	t.Run(`injection - CloudNativeFullStack`, func(t *testing.T) {
		clt := clientOneAgentInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{},
		})

		err := rec.setupOneAgentInjection(context.Background())
		require.NoError(t, err)

		var secret corev1.Secret
		err = clt.Get(context.Background(), client.ObjectKey{Name: consts.AgentInitSecretName, Namespace: testNamespace}, &secret)
		require.NoError(t, err)

		var config startup.SecretConfig
		err = json.Unmarshal(secret.Data["config"], &config)
		require.NoError(t, err)
		assert.Equal(t, testAPIToken, config.ApiToken)
		assert.Equal(t, testPaasToken, config.PaasToken)

		assertSecretNotFound(t, clt, consts.AgentInitSecretName, testNamespace2)
	})
}

func TestSetupEnrichmentInjection(t *testing.T) {
	t.Run(`no enrichment injection`, func(t *testing.T) {
		clt := clientNoInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{},
		})
		rec.dynakube.Spec.MetadataEnrichment.Enabled = false

		err := rec.setupEnrichmentInjection(context.Background())
		require.NoError(t, err)

		assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace2)
	})

	t.Run(`enrichment injection`, func(t *testing.T) {
		clt := clientEnrichmentInjection()
		rec := createReconciler(clt, testDynakube, testNamespaceDynatrace, dynatracev1beta2.OneAgentSpec{
			CloudNativeFullStack: &dynatracev1beta2.CloudNativeFullStackSpec{},
		})
		rec.dynakube.Spec.MetadataEnrichment.Enabled = true

		err := rec.setupEnrichmentInjection(context.Background())
		require.NoError(t, err)

		assertSecretFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace)
		assertSecretNotFound(t, clt, consts.EnrichmentEndpointSecretName, testNamespace2)
	})
}

func newIstioTestingClient(fakeClient *fakeistio.Clientset, dynakube *dynatracev1beta2.DynaKube) *istio.Client {
	return &istio.Client{
		IstioClientset: fakeClient,
		Owner:          dynakube,
	}
}

func createReconciler(clt client.Client, dynakubeName string, dynakubeNamespace string, oneAgentSpec dynatracev1beta2.OneAgentSpec) reconciler {
	return reconciler{
		client:    clt,
		apiReader: clt,
		dynakube: &dynatracev1beta2.DynaKube{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dynakubeName,
				Namespace: dynakubeNamespace,
			},
			Spec: dynatracev1beta2.DynaKubeSpec{
				APIURL:      testApiUrl,
				OneAgent:    oneAgentSpec,
				EnableIstio: true,
			},
		},
	}
}

func clientRemoveAppInjection() client.Client {
	return fake.NewClientWithIndex(
		clientInjectedNamespace(testNamespace, testDynakube),
		clientInjectedNamespace(testNamespace2, testDynakube2),
		clientSecret(consts.EnrichmentEndpointSecretName, testNamespace, nil),
		clientSecret(consts.EnrichmentEndpointSecretName, testNamespace2, nil),
	)
}

func clientNoInjection() client.Client {
	return fake.NewClientWithIndex(
		clientInjectedNamespace(testNamespace, testDynakube),
		clientInjectedNamespace(testNamespace2, testDynakube2),
	)
}

func clientOneAgentInjection() client.Client {
	return fake.NewClientWithIndex(
		clientInjectedNamespace(testNamespace, testDynakube),
		clientInjectedNamespace(testNamespace2, testDynakube2),
		clientSecret(testDynakube, testNamespaceDynatrace, map[string][]byte{
			dtclient.ApiToken:  []byte(testAPIToken),
			dtclient.PaasToken: []byte(testPaasToken),
		}),
	)
}

func clientEnrichmentInjection() client.Client {
	return fake.NewClientWithIndex(
		clientInjectedNamespace(testNamespace, testDynakube),
		clientInjectedNamespace(testNamespace2, testDynakube2),
		clientSecret(testDynakube, testNamespaceDynatrace, map[string][]byte{
			dtclient.ApiToken:        []byte(testAPIToken),
			dtclient.PaasToken:       []byte(testPaasToken),
			dtclient.DataIngestToken: []byte(testDataIngestToken),
		}),
	)
}

func setupIstioClientWithObjects(dynakube *dynatracev1beta2.DynaKube) *istio.Client {
	return newIstioTestingClient(fakeistio.NewSimpleClientset(
		clientServiceEntry(istio.BuildNameForIPServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace),
		clientServiceEntry(istio.BuildNameForFQDNServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace),
		clientServiceEntry(istio.BuildNameForIPServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace2),
		clientServiceEntry(istio.BuildNameForFQDNServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace2),

		clientVirtualService(istio.BuildNameForFQDNServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace),
		clientVirtualService(istio.BuildNameForFQDNServiceEntry(dynakube.Name, istio.OneAgentComponent), testNamespace2),
	), dynakube)
}

func clientInjectedNamespace(namespaceName string, dynakubeName string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "corev1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				dtwebhook.InjectionInstanceLabel: dynakubeName,
			},
		},
	}
}

func clientNotInjectedNamespace(namespaceName string, dynakubeName string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "corev1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				testNamespaceSelectorLabel: dynakubeName,
			},
		},
	}
}

func clientSecret(secretName string, namespaceName string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core/v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespaceName,
		},
		Data: data,
	}
}

func clientServiceEntry(name string, namespaceName string) *istiov1beta1.ServiceEntry {
	return &istiov1beta1.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespaceName,
		},
		Spec: istio2.ServiceEntry{},
	}
}

func clientVirtualService(name string, namespaceName string) *istiov1beta1.VirtualService {
	return &istiov1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespaceName,
		},
		Spec: istio2.VirtualService{},
	}
}

func assertSecretFound(t *testing.T, clt client.Client, secretName string, secretNamespace string) {
	var secret corev1.Secret
	err := clt.Get(context.Background(), client.ObjectKey{Name: secretName, Namespace: secretNamespace}, &secret)
	require.NoError(t, err, "%s.%s secret not found, error: %s", secretName, secretNamespace, err)
}

func assertSecretNotFound(t *testing.T, clt client.Client, secretName string, secretNamespace string) {
	var secret corev1.Secret
	err := clt.Get(context.Background(), client.ObjectKey{Name: secretName, Namespace: secretNamespace}, &secret)
	require.Error(t, err, "%s.%s secret found, error: %s ", secretName, secretNamespace, err)
	assert.True(t, k8serrors.IsNotFound(err), "%s.%s secret, unexpected error: %s", secretName, secretNamespace, err)
}

func createGenericReconcilerMock(t *testing.T) controllers.Reconciler {
	connectionInfoReconciler := controllermock.NewReconciler(t)
	connectionInfoReconciler.On("Reconcile",
		mock.AnythingOfType("context.backgroundCtx")).Return(nil).Maybe()

	return connectionInfoReconciler
}

func createVersionReconcilerMock(t *testing.T) versions.Reconciler {
	versionReconciler := versionmock.NewReconciler(t)
	versionReconciler.On("ReconcileCodeModules",
		mock.AnythingOfType("context.backgroundCtx"),
		mock.AnythingOfType("*dynakube.DynaKube")).Return(nil).Once()

	return versionReconciler
}
