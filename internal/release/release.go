package release

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource"
)

func NewRelease(name, namespace string, revision int, values map[string]interface{}, legacyChart *chart.Chart, hookResources []*resource.HookResource, generalResources []*resource.GeneralResource, notes string, opts ReleaseOptions) (*Release, error) {
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	sort.SliceStable(hookResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(hookResources[i].ResourceID, hookResources[j].ResourceID)
	})

	sort.SliceStable(generalResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(generalResources[i].ResourceID, generalResources[j].ResourceID)
	})

	var status helmrelease.Status
	if opts.Status == "" {
		status = helmrelease.StatusUnknown
	} else {
		status = opts.Status
	}

	notes = strings.TrimRightFunc(notes, unicode.IsSpace)

	if opts.InfoAnnotations == nil {
		opts.InfoAnnotations = map[string]string{}
	}

	return &Release{
		name:             name,
		namespace:        namespace,
		revision:         revision,
		values:           values,
		legacyChart:      legacyChart,
		mapper:           opts.Mapper,
		status:           status,
		firstDeployed:    opts.FirstDeployed,
		lastDeployed:     opts.LastDeployed,
		appVersion:       legacyChart.Metadata.AppVersion,
		chartName:        legacyChart.Metadata.Name,
		chartVersion:     legacyChart.Metadata.Version,
		infoAnnotations:  opts.InfoAnnotations,
		hookResources:    hookResources,
		generalResources: generalResources,
		notes:            notes,
	}, nil
}

type ReleaseOptions struct {
	InfoAnnotations map[string]string
	Status          helmrelease.Status
	FirstDeployed   time.Time
	LastDeployed    time.Time
	Mapper          meta.ResettableRESTMapper
}

func NewReleaseFromLegacyRelease(legacyRelease *helmrelease.Release, opts ReleaseFromLegacyReleaseOptions) (*Release, error) {
	var hookResources []*resource.HookResource
	for _, legacyHook := range legacyRelease.Hooks {
		if res, err := resource.NewHookResourceFromManifest(legacyHook.Manifest, resource.HookResourceFromManifestOptions{
			FilePath:         legacyHook.Path,
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing hook resource from manifest for legacy release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			hookResources = append(hookResources, res)
		}
	}

	var generalResources []*resource.GeneralResource
	for _, manifest := range releaseutil.SplitManifests(legacyRelease.Manifest) {
		if res, err := resource.NewGeneralResourceFromManifest(manifest, resource.GeneralResourceFromManifestOptions{
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing general resource from manifest for legacy release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			generalResources = append(generalResources, res)
		}
	}

	rel, err := NewRelease(legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, legacyRelease.Config, legacyRelease.Chart, hookResources, generalResources, legacyRelease.Info.Notes, ReleaseOptions{
		InfoAnnotations: legacyRelease.Info.Annotations,
		Status:          legacyRelease.Info.Status,
		FirstDeployed:   legacyRelease.Info.FirstDeployed.Time,
		LastDeployed:    legacyRelease.Info.LastDeployed.Time,
		Mapper:          opts.Mapper,
	})
	if err != nil {
		return nil, fmt.Errorf("error building release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
	}

	return rel, nil
}

type ReleaseFromLegacyReleaseOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type Release struct {
	name        string
	namespace   string
	revision    int
	values      map[string]interface{}
	legacyChart *chart.Chart
	mapper      meta.ResettableRESTMapper

	status          helmrelease.Status
	firstDeployed   time.Time
	lastDeployed    time.Time
	appVersion      string
	chartName       string
	chartVersion    string
	infoAnnotations map[string]string

	hookResources    []*resource.HookResource
	generalResources []*resource.GeneralResource
	notes            string
}

func (r *Release) Name() string {
	return r.name
}

func (r *Release) Namespace() string {
	return r.namespace
}

func (r *Release) Revision() int {
	return r.revision
}

func (r *Release) Values() map[string]interface{} {
	return r.values
}

func (r *Release) LegacyChart() *chart.Chart {
	return r.legacyChart
}

func (r *Release) HookResources() []*resource.HookResource {
	return r.hookResources
}

func (r *Release) GeneralResources() []*resource.GeneralResource {
	return r.generalResources
}

func (r *Release) Notes() string {
	return r.notes
}

func (r *Release) Status() helmrelease.Status {
	return r.status
}

func (r *Release) FirstDeployed() time.Time {
	return r.firstDeployed
}

func (r *Release) LastDeployed() time.Time {
	return r.lastDeployed
}

func (r *Release) AppVersion() string {
	return r.appVersion
}

func (r *Release) ChartName() string {
	return r.chartName
}

func (r *Release) ChartVersion() string {
	return r.chartVersion
}

func (r *Release) InfoAnnotations() map[string]string {
	return r.infoAnnotations
}

func (r *Release) ID() string {
	return fmt.Sprintf("%s:%s:%d", r.namespace, r.name, r.revision)
}

func (r *Release) HumanID() string {
	return fmt.Sprintf("%s:%s/%d", r.namespace, r.name, r.revision)
}

func (r *Release) Fail() {
	r.status = helmrelease.StatusFailed
}

func (r *Release) Supersede() {
	r.status = helmrelease.StatusSuperseded
}

func (r *Release) Succeed() {
	r.status = helmrelease.StatusDeployed
}

func (r *Release) Succeeded() bool {
	switch r.status {
	case helmrelease.StatusDeployed,
		helmrelease.StatusSuperseded,
		helmrelease.StatusUninstalled:
		return true
	}

	return false
}

func (r *Release) Failed() bool {
	switch r.status {
	case helmrelease.StatusFailed,
		helmrelease.StatusUnknown,
		helmrelease.StatusPendingInstall,
		helmrelease.StatusPendingUpgrade,
		helmrelease.StatusPendingRollback,
		helmrelease.StatusUninstalling:
		return true
	}

	return false
}

func (r *Release) Pend(deployType common.DeployType) {
	r.status = helmrelease.StatusPendingInstall

	switch deployType {
	case common.DeployTypeInitial,
		common.DeployTypeInstall:
		r.status = helmrelease.StatusPendingInstall
	case common.DeployTypeUpgrade:
		r.status = helmrelease.StatusPendingUpgrade
	case common.DeployTypeRollback:
		r.status = helmrelease.StatusPendingRollback
	}

	now := time.Now()
	if r.firstDeployed.IsZero() {
		r.firstDeployed = now
	}
	r.lastDeployed = now
}

func (r *Release) Skip() {
	r.status = helmrelease.StatusSkipped
}
