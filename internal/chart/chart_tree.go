package chart

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/cli/values"
	"github.com/werf/3p-helm/pkg/downloader"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/internal/resource"
)

func NewChartTree(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, actionConfig *action.Configuration, opts ChartTreeOptions) (*ChartTree, error) {
	valOpts := &values.Options{
		StringValues: opts.StringSetValues,
		Values:       opts.SetValues,
		FileValues:   opts.FileValues,
		ValueFiles:   opts.ValuesFiles,
	}

	getters := getter.All(helm_v3.Settings)

	log.Default.Debug(ctx, "Merging values for chart tree at %q", chartPath)
	releaseValues, err := valOpts.MergeValues(getters)
	if err != nil {
		return nil, fmt.Errorf("error merging values for chart tree at %q: %w", chartPath, err)
	}

	log.Default.Debug(ctx, "Loading chart at %q", chartPath)
	legacyChart, err := loader.Load(chartPath)
	if err != nil {
		var e *downloader.ErrRepoNotFound
		if errors.As(err, &e) {
			return nil, fmt.Errorf("%w. Please add the missing repos via 'helm repo add'", e)
		}

		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, err)
	} else if legacyChart == nil {
		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, action.ErrMissingChart())
	} else if legacyChart.Metadata.Type != "" && legacyChart.Metadata.Type != "application" {
		return nil, fmt.Errorf("chart %q of type %q can't be deployed", legacyChart.Name(), legacyChart.Metadata.Type)
	} else if legacyChart.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(legacyChart, legacyChart.Metadata.Dependencies); err != nil {
			return nil, fmt.Errorf("error while checking chart dependencies for chart %q: %w", legacyChart.Name(), err)
		}
	}

	if err := chartutil.ProcessDependenciesWithMerge(legacyChart, &releaseValues); err != nil {
		return nil, fmt.Errorf("error processing chart %q dependencies: %w", legacyChart.Name(), err)
	}

	if legacyChart.Metadata.Deprecated {
		log.Default.Warn(ctx, `Chart "%s:%s" is deprecated`, legacyChart.Name(), legacyChart.Metadata.Version)
	}

	caps, err := actionConfig.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities for chart %q: %w", legacyChart.Name(), err)
	}

	var isUpgrade bool
	switch deployType {
	case common.DeployTypeUpgrade, common.DeployTypeRollback:
		isUpgrade = true
	case common.DeployTypeInitial, common.DeployTypeInstall:
		isUpgrade = false
	}

	log.Default.Debug(ctx, "Rendering values for chart at %q", chartPath)
	values, err := chartutil.ToRenderValues(legacyChart, releaseValues, chartutil.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps)
	if err != nil {
		return nil, fmt.Errorf("error building values for chart %q: %w", legacyChart.Name(), err)
	}

	finalValues := values.AsMap()
	hasClusterAccess := opts.Mapper != nil

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)
	legacyHookResources, generalManifestsBuf, notes, err := actionConfig.RenderResources(legacyChart, values, "", "", opts.SubNotes, false, false, nil, hasClusterAccess, false)
	if err != nil {
		log.Default.Debug(ctx, generalManifestsBuf.String())

		return nil, fmt.Errorf("error rendering resources for chart %q: %w", legacyChart.Name(), err)
	}

	notes = strings.TrimRightFunc(notes, unicode.IsSpace)

	var standaloneCRDs []*resource.StandaloneCRD
	for _, crd := range legacyChart.CRDObjects() {
		for _, manifest := range releaseutil.SplitManifests(string(crd.File.Data)) {
			if res, err := resource.NewStandaloneCRDFromManifest(manifest, resource.StandaloneCRDFromManifestOptions{
				FilePath:         crd.Filename,
				DefaultNamespace: releaseNamespace,
				Mapper:           opts.Mapper,
			}); err != nil {
				return nil, fmt.Errorf("error constructing standalone CRD for chart at %q: %w", chartPath, err)
			} else {
				standaloneCRDs = append(standaloneCRDs, res)
			}
		}
	}

	sort.SliceStable(standaloneCRDs, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(standaloneCRDs[i].ResourceID, standaloneCRDs[j].ResourceID)
	})

	var hookResources []*resource.HookResource
	for _, hook := range legacyHookResources {
		for _, manifest := range releaseutil.SplitManifests(hook.Manifest) {
			if res, err := resource.NewHookResourceFromManifest(manifest, resource.HookResourceFromManifestOptions{
				DefaultNamespace: releaseNamespace,
				Mapper:           opts.Mapper,
				DiscoveryClient:  opts.DiscoveryClient,
				FilePath:         hook.Path,
			}); err != nil {
				return nil, fmt.Errorf("error constructing hook resource for chart at %q: %w", chartPath, err)
			} else {
				hookResources = append(hookResources, res)
			}
		}
	}

	sort.SliceStable(hookResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(hookResources[i].ResourceID, hookResources[j].ResourceID)
	})

	var generalResources []*resource.GeneralResource
	for _, manifest := range releaseutil.SplitManifests(generalManifestsBuf.String()) {
		if res, err := resource.NewGeneralResourceFromManifest(manifest, resource.GeneralResourceFromManifestOptions{
			DefaultNamespace: releaseNamespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing general resource for chart at %q: %w", chartPath, err)
		} else {
			generalResources = append(generalResources, res)
		}
	}

	sort.SliceStable(generalResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(generalResources[i].ResourceID, generalResources[j].ResourceID)
	})

	return &ChartTree{
		standaloneCRDs:   standaloneCRDs,
		hookResources:    hookResources,
		generalResources: generalResources,
		notes:            notes,
		releaseValues:    releaseValues,
		finalValues:      finalValues,
		legacyChart:      legacyChart,
	}, nil
}

type ChartTreeOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
	StringSetValues []string
	SetValues       []string
	FileValues      []string
	ValuesFiles     []string
	SubNotes        bool
}

type ChartTree struct {
	standaloneCRDs   []*resource.StandaloneCRD
	hookResources    []*resource.HookResource
	generalResources []*resource.GeneralResource
	notes            string
	releaseValues    map[string]interface{}
	finalValues      map[string]interface{}
	legacyChart      *chart.Chart
}

func (t *ChartTree) Name() string {
	return t.legacyChart.Name()
}

func (t *ChartTree) Path() string {
	return t.legacyChart.ChartFullPath()
}

func (t *ChartTree) StandaloneCRDs() []*resource.StandaloneCRD {
	return t.standaloneCRDs
}

func (t *ChartTree) HookResources() []*resource.HookResource {
	return t.hookResources
}

func (t *ChartTree) GeneralResources() []*resource.GeneralResource {
	return t.generalResources
}

func (t *ChartTree) Notes() string {
	return t.notes
}

func (t *ChartTree) ReleaseValues() map[string]interface{} {
	return t.releaseValues
}

func (t *ChartTree) FinalValues() map[string]interface{} {
	return t.finalValues
}

func (t *ChartTree) LegacyChart() *chart.Chart {
	return t.legacyChart
}
