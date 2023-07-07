package init

import (
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	ctrl "sigs.k8s.io/controller-runtime"
)

const use = "csi-init"

type CommandBuilder struct {
}

type CsiInit struct{}

func (init *CsiInit) Init(_ logr.RuntimeInfo) {}

func (init *CsiInit) Enabled(_ int) bool {
	return true
}

func (init *CsiInit) Info(_ int, msg string, keysAndValues ...interface{}) {
	fmt.Print(msg, keysAndValues)
}

func (init *CsiInit) Error(err error, msg string, keysAndValues ...interface{}) {
	fmt.Print(err, msg, keysAndValues)
}

func (init *CsiInit) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &CsiInit{}
}

func (init *CsiInit) WithName(name string) logr.LogSink {
	return &CsiInit{}
}

func NewCommandBuilder() CommandBuilder {
	return CommandBuilder{}
}

func (builder CommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:  use,
		Long: "makes the bed for the csi-driver",
		RunE: builder.buildRun(),
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

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

		err = csiManager.Start(signalHandler)
		return errors.WithStack(err)
	}
}
