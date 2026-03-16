package helm

import (
	"fmt"
	"path"
	"strings"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

type OCILoader struct{}

func (o *OCILoader) LoadTags(baseURL string) (*repo.IndexFile, error) {
	ref := strings.TrimPrefix(baseURL, "oci://")

	client, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI registry client: %w", err)
	}

	tags, err := client.Tags(ref)
	if err != nil {
		return nil, fmt.Errorf("could not list tags for %s: %w", baseURL, err)
	}

	ref = strings.TrimSuffix(ref, "/")
	if ref == "" {
		return nil, fmt.Errorf("invalid OCI reference: %s", baseURL)
	}
	chartName := path.Base(ref)

	indexFile := repo.NewIndexFile()
	versions := make(repo.ChartVersions, 0, len(tags))
	for _, tag := range tags {
		versions = append(versions, &repo.ChartVersion{
			Metadata: &chart.Metadata{
				Name:    chartName,
				Version: tag,
			},
			URLs:    []string{baseURL},
			Created: time.Now(),
		})
	}
	indexFile.Entries[chartName] = versions
	return indexFile, nil
}
