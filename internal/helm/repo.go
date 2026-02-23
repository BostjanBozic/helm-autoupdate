package helm

import (
	"fmt"
	"net/url"
	"sync"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

type IndexLoader interface {
	LoadIndexFile(URL string, chart *AutoUpdateChart) (*repo.IndexFile, error)
}

type DefaultProviders struct {
	Providers getter.Providers
	mu        sync.Mutex
}

func (r *DefaultProviders) getProviders() getter.Providers {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Providers != nil {
		return r.Providers
	}
	r.Providers = getter.All(cli.New())
	return r.Providers
}

type DirectLoader struct {
	DefaultProviders
	s3 s3Getter
}

func parseIndexFile(data []byte) (*repo.IndexFile, error) {
	var indexFile repo.IndexFile
	if err := yaml.UnmarshalStrict(data, &indexFile); err != nil {
		return nil, fmt.Errorf("failed to parse index.yaml: %w", err)
	}
	return &indexFile, nil
}

func (r *DirectLoader) LoadIndexFile(baseURL string, chart *AutoUpdateChart) (*repo.IndexFile, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid chart baseURL format: %s", baseURL)
	}

	if u.Scheme == "oci" {
		var ociLoader OCILoader
		return ociLoader.LoadTags(baseURL)
	}

	if u.Scheme == "s3" {
		region := ""
		if chart != nil {
			region = chart.S3Region
		}
		content, err := r.s3.GetWithRegion(baseURL+"/index.yaml", region)
		if err != nil {
			return nil, fmt.Errorf("could not fetch index file for %s: %w", baseURL, err)
		}
		return parseIndexFile(content.Bytes())
	}

	client, err := r.getProviders().ByScheme(u.Scheme)
	if err != nil {
		return nil, fmt.Errorf("could not find protocol handler for: %s", u.Scheme)
	}
	indexURL := baseURL + "/index.yaml"
	content, err := client.Get(indexURL, getter.WithURL(indexURL))
	if err != nil {
		return nil, fmt.Errorf("could not fetch index file for %s: %w", baseURL, err)
	}
	if content == nil {
		return nil, fmt.Errorf("no content for %s", indexURL)
	}
	return parseIndexFile(content.Bytes())
}

type CachedLoader struct {
	IndexLoader IndexLoader
	cache       map[string]*repo.IndexFile
	mu          sync.Mutex
}

func (r *CachedLoader) LoadIndexFile(indexURL string, chart *AutoUpdateChart) (*repo.IndexFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = make(map[string]*repo.IndexFile)
	}
	if indexFile, ok := r.cache[indexURL]; ok {
		return indexFile, nil
	}
	indexFile, err := r.IndexLoader.LoadIndexFile(indexURL, chart)
	if err != nil {
		return nil, fmt.Errorf("failed to load index file for %s: %w", indexURL, err)
	}
	r.cache[indexURL] = indexFile
	return indexFile, nil
}
