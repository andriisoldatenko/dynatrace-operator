package csigc

import (
	"context"
	"os"
	"time"

	dynatracev1beta1 "github.com/Dynatrace/dynatrace-operator/src/api/v1beta1"
	dtcsi "github.com/Dynatrace/dynatrace-operator/src/controllers/csi"
	"github.com/Dynatrace/dynatrace-operator/src/controllers/csi/metadata"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// can contain the tag of the image or the digest, depending on how the user provided the image
// or the version set for the download
type pinnedVersionSet map[string]bool

func (set pinnedVersionSet) isNotPinned(version string) bool {
	return !set[version]
}

// garbageCollectionInfo stores tenant specific information
// used to delete unused files or directories connected to that tenant
type garbageCollectionInfo struct {
	tenantUUID     string
	pinnedVersions pinnedVersionSet
}

// CSIGarbageCollector removes unused and outdated agent versions
type CSIGarbageCollector struct {
	apiReader client.Reader
	fs        afero.Fs
	db        metadata.Access
	path      metadata.PathResolver

	maxUnmountedVolumeAge time.Duration
}

var _ reconcile.Reconciler = (*CSIGarbageCollector)(nil)

// NewCSIGarbageCollector returns a new CSIGarbageCollector
func NewCSIGarbageCollector(apiReader client.Reader, opts dtcsi.CSIOptions, db metadata.Access) *CSIGarbageCollector {
	return &CSIGarbageCollector{
		apiReader:             apiReader,
		fs:                    afero.NewOsFs(),
		db:                    db,
		path:                  metadata.PathResolver{RootDir: opts.RootDir},
		maxUnmountedVolumeAge: determineMaxUnmountedVolumeAge(os.Getenv(maxUnmountedCsiVolumeAgeEnv)),
	}
}

func (gc *CSIGarbageCollector) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log.Info("running OneAgent garbage collection", "namespace", request.Namespace, "name", request.Name)
	defaultReconcileResult := reconcile.Result{}

	dynakube, err := getDynakubeFromRequest(ctx, gc.apiReader, request)
	if err != nil {
		return defaultReconcileResult, err
	}
	if dynakube == nil {
		return defaultReconcileResult, nil
	}

	if !dynakube.NeedAppInjection() {
		log.Info("app injection not enabled, skip garbage collection", "dynakube", dynakube.Name)
		return defaultReconcileResult, nil
	}

	gcInfo := collectGCInfo(*dynakube)
	if gcInfo == nil {
		return defaultReconcileResult, nil
	}

	log.Info("running binary garbage collection")
	gc.runBinaryGarbageCollection(ctx, gcInfo.tenantUUID)

	if err := ctx.Err(); err != nil {
		return defaultReconcileResult, err
	}

	log.Info("running log garbage collection")
	gc.runUnmountedVolumeGarbageCollection(gcInfo.tenantUUID)

	if err := ctx.Err(); err != nil {
		return defaultReconcileResult, err
	}

	log.Info("running shared images garbage collection")
	if err := gc.runSharedImagesGarbageCollection(ctx); err != nil {
		log.Info("failed to garbage collect the shared images")
		return defaultReconcileResult, err
	}

	return defaultReconcileResult, nil
}

func getDynakubeFromRequest(ctx context.Context, apiReader client.Reader, request reconcile.Request) (*dynatracev1beta1.DynaKube, error) {
	var dynakube dynatracev1beta1.DynaKube
	if err := apiReader.Get(ctx, request.NamespacedName, &dynakube); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("given DynaKube object not found")
			return nil, nil
		}

		log.Info("failed to get DynaKube object")
		return nil, errors.WithStack(err)
	}
	return &dynakube, nil
}

func collectGCInfo(dynakube dynatracev1beta1.DynaKube) *garbageCollectionInfo {
	tenantUUID, err := dynakube.TenantUUIDFromApiUrl()
	if err != nil {
		log.Info("failed to get tenantUUID of DynaKube, checking later")
		return nil
	}

	return &garbageCollectionInfo{
		tenantUUID:     tenantUUID,
	}
}
