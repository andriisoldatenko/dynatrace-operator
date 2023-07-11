package init

import (
	"github.com/Dynatrace/dynatrace-operator/src/cmd/config"
	cmdManager "github.com/Dynatrace/dynatrace-operator/src/cmd/manager"
	dtcsi "github.com/Dynatrace/dynatrace-operator/src/controllers/csi"
	"github.com/Dynatrace/dynatrace-operator/src/controllers/csi/metadata"
	"github.com/Dynatrace/dynatrace-operator/src/version"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	ctrl "sigs.k8s.io/controller-runtime"
)

const use = "csi-init"

var (
	nodeId   = ""
	endpoint = ""
)

type CommandBuilder struct {
	configProvider  config.Provider
	managerProvider cmdManager.Provider
	namespace       string
	filesystem      afero.Fs
	csiOptions      *dtcsi.CSIOptions
}

func NewCsiInitCommandBuilder() CommandBuilder {
	return CommandBuilder{}
}

func (builder CommandBuilder) SetConfigProvider(provider config.Provider) CommandBuilder {
	builder.configProvider = provider
	return builder
}

func (builder CommandBuilder) SetNamespace(namespace string) CommandBuilder {
	builder.namespace = namespace
	return builder
}

func (builder CommandBuilder) getCsiOptions() dtcsi.CSIOptions {
	if builder.csiOptions == nil {
		builder.csiOptions = &dtcsi.CSIOptions{
			NodeId:   nodeId,
			Endpoint: endpoint,
			RootDir:  dtcsi.DataPath,
		}
	}

	return *builder.csiOptions
}

func (builder CommandBuilder) getManagerProvider() cmdManager.Provider {
	if builder.managerProvider == nil {
		builder.managerProvider = newCsiInitManagerProvider()
	}

	return builder.managerProvider
}

func (builder CommandBuilder) getFilesystem() afero.Fs {
	if builder.filesystem == nil {
		builder.filesystem = afero.NewOsFs()
	}

	return builder.filesystem
}

func (builder CommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:  use,
		RunE: builder.buildRun(),
	}

	return cmd
}

func (builder CommandBuilder) buildRun() func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		unix.Umask(0000)
		version.LogVersion()

		kubeConfig, err := builder.configProvider.GetConfig()
		if err != nil {
			return err
		}

		csiManager, err := builder.getManagerProvider().CreateManager(builder.namespace, kubeConfig)
		if err != nil {
			return err
		}

		err = createCsiDataPath(builder.getFilesystem())
		if err != nil {
			return err
		}

		signalHandler := ctrl.SetupSignalHandler()
		access, err := metadata.NewAccess(signalHandler, dtcsi.MetadataAccessPath)
		if err != nil {
			return err
		}

		err = metadata.NewCorrectnessChecker(csiManager.GetClient(), access, builder.getCsiOptions()).CorrectCSI(signalHandler)
		if err != nil {
			return err
		}
		return nil
	}
}

func createCsiDataPath(fs afero.Fs) error {
	return errors.WithStack(fs.MkdirAll(dtcsi.DataPath, 0770))
}
