<!-- Code generated by ./hack/doc/gen_feature_flags.sh - DO NOT EDIT -->

# dynakube

```go
import "github.com/Dynatrace/dynatrace-operator/pkg/api/v1beta2/dynakube/tmp"
```

## Index

- [Constants](<#constants>)

## Constants

<a name="AnnotationFeaturePrefix"></a>

```go
const (
    AnnotationFeaturePrefix = "feature.dynatrace.com/"

    // General.
    AnnotationFeaturePublicRegistry = AnnotationFeaturePrefix + "public-registry"

    // Deprecated: AnnotationFeatureDisableActiveGateUpdates use AnnotationFeatureActiveGateUpdates instead.
    AnnotationFeatureDisableActiveGateUpdates = AnnotationFeaturePrefix + "disable-activegate-updates"

    AnnotationFeatureActiveGateUpdates = AnnotationFeaturePrefix + "activegate-updates"

    AnnotationFeatureActiveGateAppArmor                   = AnnotationFeaturePrefix + "activegate-apparmor"
    AnnotationFeatureAutomaticK8sApiMonitoring            = AnnotationFeaturePrefix + "automatic-kubernetes-api-monitoring"
    AnnotationFeatureAutomaticK8sApiMonitoringClusterName = AnnotationFeaturePrefix + "automatic-kubernetes-api-monitoring-cluster-name"
    AnnotationFeatureK8sAppEnabled                        = AnnotationFeaturePrefix + "k8s-app-enabled"
    AnnotationFeatureActiveGateIgnoreProxy                = AnnotationFeaturePrefix + "activegate-ignore-proxy"

    AnnotationFeatureNoProxy = AnnotationFeaturePrefix + "no-proxy"

    AnnotationFeatureMultipleOsAgentsOnNode         = AnnotationFeaturePrefix + "multiple-osagents-on-node"
    AnnotationFeatureOneAgentMaxUnavailable         = AnnotationFeaturePrefix + "oneagent-max-unavailable"
    AnnotationFeatureOneAgentIgnoreProxy            = AnnotationFeaturePrefix + "oneagent-ignore-proxy"
    AnnotationFeatureOneAgentInitialConnectRetry    = AnnotationFeaturePrefix + "oneagent-initial-connect-retry-ms"
    AnnotationFeatureRunOneAgentContainerPrivileged = AnnotationFeaturePrefix + "oneagent-privileged"

    AnnotationFeatureIgnoreUnknownState    = AnnotationFeaturePrefix + "ignore-unknown-state"
    AnnotationFeatureIgnoredNamespaces     = AnnotationFeaturePrefix + "ignored-namespaces"
    AnnotationFeatureAutomaticInjection    = AnnotationFeaturePrefix + "automatic-injection"
    AnnotationFeatureLabelVersionDetection = AnnotationFeaturePrefix + "label-version-detection"
    AnnotationInjectionFailurePolicy       = AnnotationFeaturePrefix + "injection-failure-policy"
    AnnotationFeatureInitContainerSeccomp  = AnnotationFeaturePrefix + "init-container-seccomp-profile"
    AnnotationFeatureEnforcementMode       = AnnotationFeaturePrefix + "enforcement-mode"

    // CSI.
    AnnotationFeatureMaxFailedCsiMountAttempts = AnnotationFeaturePrefix + "max-csi-mount-attempts"
    AnnotationFeatureReadOnlyCsiVolume         = AnnotationFeaturePrefix + "injection-readonly-volume"
)
```

<a name="DefaultMaxFailedCsiMountAttempts"></a>

```go
const (
    DefaultMaxFailedCsiMountAttempts        = 10
    DefaultMinRequestThresholdMinutes       = 15
    IstioDefaultOneAgentInitialConnectRetry = 6000
)
```
