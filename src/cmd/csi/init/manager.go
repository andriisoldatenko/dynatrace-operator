package init

import (
	cmdManager "github.com/Dynatrace/dynatrace-operator/src/cmd/manager"
	"github.com/Dynatrace/dynatrace-operator/src/scheme"
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type csiInitManagerProvider struct{}

func newCsiInitManagerProvider() cmdManager.Provider {
	return csiInitManagerProvider{}
}

func (provider csiInitManagerProvider) CreateManager(namespace string, config *rest.Config) (manager.Manager, error) {
	mgr, err := manager.New(config, provider.createOptions(namespace))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err != nil {
		return nil, err
	}

	return mgr, nil
}

func (provider csiInitManagerProvider) createOptions(namespace string) ctrl.Options {
	return ctrl.Options{
		Namespace: namespace,
		Scheme:    scheme.Scheme,
	}
}
