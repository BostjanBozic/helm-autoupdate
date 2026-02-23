package helm

import (
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
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

	// Chart name from the last path segment.
	parts := strings.Split(ref, "/")
	chartName := parts[len(parts)-1]

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
