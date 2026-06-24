package helm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	"sigs.k8s.io/yaml"
)

func TestParseLine(t *testing.T) {
	require.Nil(t, ParseLine("asdfdasdsfadsa"))
	require.Equal(t, &LineParse{
		Prefix:         "  version",
		CurrentVersion: "0.3.6",
		Identity:       "datadog",
		Suffix:         "",
	}, ParseLine("  version: 0.3.6 # helm:autoupdate:datadog"))
}

func TestParseLine_String(t *testing.T) {
	require.Equal(t, "  version: 0.3.6 # helm:autoupdate:datadog", ParseLine("  version: 0.3.6 # helm:autoupdate:datadog").String())
}

const (
	identityAWSVPCCNI = "aws-vpc-cni"
	chartNameTest     = "testchart"
)

const cniFile = `apiVersion: toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: aws-vpc-cni
spec:
  chart:
    spec:
      chart: aws-vpc-cni
      sourceRef:
        kind: HelmRepository
        name: aws-vpc-cni
      version: 0.3.6 # helm:autoupdate:aws-vpc-cni
  interval: 1m0s
  timeout: 10m0s # Lots of pods in the daemonset
  values:
    a: b
`

func cniFileMatchesExpected(t *testing.T, pf *ParsedFile) {
	t.Helper()
	require.Equal(t, cniFile, pf.OriginalContent)
	require.Equal(t, "  chart:", pf.Lines[5])
	require.Equal(t, []Update{
		{
			LineNumber: 11,
			Parse: &LineParse{
				Prefix:         "      version",
				CurrentVersion: "0.3.6",
				Identity:       identityAWSVPCCNI,
				Suffix:         "",
			},
		},
	}, pf.RequestedUpdates)
}

func TestParseContent(t *testing.T) {
	pf := ParseContent(cniFile)
	cniFileMatchesExpected(t, &pf)
}

func TestParseFile(t *testing.T) {
	f, err := os.CreateTemp("", "TestParseFile")
	require.NoError(t, err)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			panic(err)
		}
	}(f.Name())
	require.NoError(t, os.WriteFile(f.Name(), []byte(cniFile), 0600))
	pf, err := ParseFile(f.Name())
	require.NoError(t, err)
	cniFileMatchesExpected(t, pf)
}

func TestApplyUpdate(t *testing.T) {
	pf := ParseContent(cniFile)
	pf.ApplyUpdate(&Update{
		LineNumber: 11,
		Parse: &LineParse{
			Prefix:         "      version",
			CurrentVersion: "0.0.0",
			Identity:       identityAWSVPCCNI,
			Suffix:         "",
		},
	})
	require.Contains(t, string(pf.Bytes()), "version: 0.0.0")
}

const testConfig = `charts:
- chart:
    name: aws-vpc-cni
    repository: https://aws.github.io/eks-charts
    version: 1.0.5
  identity: aws-vpc-cni
- chart:
    name: datadog
    repository: https://datadoghq.com
    version: 1.0.0
  identity: datadog
filename_regex:
- .*\.yaml
`

func TestLoadFile(t *testing.T) {
	f, err := os.CreateTemp("", "TestLoadFile")
	require.NoError(t, err)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			panic(err)
		}
	}(f.Name())
	require.NoError(t, os.WriteFile(f.Name(), []byte(testConfig), 0600))
	ac, err := LoadFile(f.Name())
	require.NoError(t, err)
	b, err := yaml.Marshal(ac)
	require.NoError(t, err)
	require.Equal(t, testConfig, string(b))
}

func generateExample(t *testing.T) (string, func()) {
	t.Helper()
	dirName, err := os.MkdirTemp("", "generateExample")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dirName, ".helm-autoupdate.yaml"), []byte(testConfig), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dirName, "aws-vpc-cni.yaml"), []byte(cniFile), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dirName, "test-example.yaml"), []byte(`name: jack`), 0600))
	return dirName, func() {
		err := os.RemoveAll(dirName)
		if err != nil {
			panic(err)
		}
	}
}

func TestFindRequestedChanges(t *testing.T) {
	dirName, cleanup := generateExample(t)
	defer cleanup()
	x := DirectorySearchForChanges{
		Dir: dirName,
	}
	changeFiles, err := x.FindRequestedChanges(nil)
	require.NoError(t, err)
	require.Len(t, changeFiles, 1)
	require.Equal(t, filepath.Join(dirName, "aws-vpc-cni.yaml"), changeFiles[0].OriginalFilename)
}

func TestWriteChangesToFilesystem(t *testing.T) {
	dirName, cleanup := generateExample(t)
	defer cleanup()
	x := DirectorySearchForChanges{
		Dir: dirName,
	}
	changeFiles, err := x.FindRequestedChanges(nil)
	require.NoError(t, err)
	ru := changeFiles[0].RequestedUpdates[0]
	ru.Parse.CurrentVersion = "99.99.99"
	changeFiles[0].ApplyUpdate(&ru)
	require.NoError(t, WriteChangesToFilesystem(changeFiles))
	b, err := os.ReadFile(filepath.Join(dirName, "aws-vpc-cni.yaml")) //nolint:gosec
	require.NoError(t, err)
	require.Contains(t, string(b), "      version: 99.99.99 # helm:autoupdate:aws-vpc-cni")
}

func TestFindUpdateChartForUpdate(t *testing.T) {
	ac, err := Load([]byte(testConfig))
	require.NoError(t, err)
	require.Nil(t, ac.findUpdateChartForUpdate(&Update{
		Parse: &LineParse{},
	}))
	x := ac.findUpdateChartForUpdate(&Update{
		Parse: &LineParse{
			Identity: "blarg",
		},
	})
	require.Nil(t, x)
	x = ac.findUpdateChartForUpdate(&Update{
		Parse: &LineParse{
			Identity: identityAWSVPCCNI,
		},
	})
	require.NotNil(t, x)
}

func TestApplyUpdatesToFiles(t *testing.T) {
	dirName, cleanup := generateExample(t)
	defer cleanup()
	ac, err := LoadFile(filepath.Join(dirName, ".helm-autoupdate.yaml"))
	require.NoError(t, err)

	var l DirectLoader
	x := DirectorySearchForChanges{
		Dir: dirName,
	}
	pf, err := x.FindRequestedChanges(nil)
	require.NoError(t, err)
	require.Len(t, pf, 1)

	updatedFiles, err := ApplyUpdatesToFiles(&l, ac, pf)
	require.NoError(t, err)
	require.Len(t, updatedFiles, 1)
	require.NoError(t, WriteChangesToFilesystem(updatedFiles))
	b, err := os.ReadFile(filepath.Join(dirName, "aws-vpc-cni.yaml")) //nolint:gosec
	require.NoError(t, err)
	require.Contains(t, string(b), "      version: 1.0.5 # helm:autoupdate:aws-vpc-cni")
}

type mockIndexLoader struct {
	indexFile *repo.IndexFile
}

func (m *mockIndexLoader) LoadIndexFile(_ string, _ *AutoUpdateChart) (*repo.IndexFile, error) {
	return m.indexFile, nil
}

type chartVersionEntry struct {
	version string
	created time.Time
}

func makeIndexFileMulti(entries ...chartVersionEntry) *repo.IndexFile {
	idx := repo.NewIndexFile()
	versions := make(repo.ChartVersions, 0, len(entries))
	for _, e := range entries {
		versions = append(versions, &repo.ChartVersion{
			Metadata: &chart.Metadata{Name: chartNameTest, Version: e.version},
			Created:  e.created,
		})
	}
	idx.Entries[chartNameTest] = versions
	return idx
}

func makeIndexFile(created time.Time) *repo.IndexFile {
	return makeIndexFileMulti(chartVersionEntry{version: "1.0.0", created: created})
}

func TestCheckForUpdate_Cooldown(t *testing.T) {
	desc := &AutoUpdateChart{
		Repository: "https://example.com",
		Name:       chartNameTest,
		Version:    "*",
	}
	request := &Update{Parse: &LineParse{CurrentVersion: "0.0.1", Identity: chartNameTest}}

	t.Run("no cooldown - recent version still updates", func(t *testing.T) {
		desc.CooldownDays = 0
		il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-1 * time.Hour))}
		result, err := CheckForUpdate(il, desc, request)
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("cooldown satisfied - version old enough", func(t *testing.T) {
		desc.CooldownDays = 7
		il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-8 * 24 * time.Hour))}
		result, err := CheckForUpdate(il, desc, request)
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("cooldown active - version too recent", func(t *testing.T) {
		desc.CooldownDays = 7
		il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-1 * time.Hour))}
		result, err := CheckForUpdate(il, desc, request)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("cooldown set but Created is zero - update proceeds (OCI)", func(t *testing.T) {
		desc.CooldownDays = 7
		il := &mockIndexLoader{indexFile: makeIndexFile(time.Time{})}
		result, err := CheckForUpdate(il, desc, request)
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("fallback to older version when latest is within cooldown", func(t *testing.T) {
		desc.CooldownDays = 7
		il := &mockIndexLoader{indexFile: makeIndexFileMulti(
			chartVersionEntry{"1.1.0", time.Now().Add(-1 * time.Hour)},      // too recent
			chartVersionEntry{"1.0.0", time.Now().Add(-8 * 24 * time.Hour)}, // past cooldown
		)}
		result, err := CheckForUpdate(il, desc, request)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "1.0.0", result.Parse.CurrentVersion)
	})

	t.Run("no downgrade when all newer versions are within cooldown", func(t *testing.T) {
		desc.CooldownDays = 7
		requestAt120 := &Update{Parse: &LineParse{CurrentVersion: "1.2.0", Identity: chartNameTest}}
		il := &mockIndexLoader{indexFile: makeIndexFileMulti(
			chartVersionEntry{"1.2.0", time.Now().Add(-1 * time.Hour)},      // too recent (same as current)
			chartVersionEntry{"1.1.0", time.Now().Add(-8 * 24 * time.Hour)}, // past cooldown but older than current
		)}
		result, err := CheckForUpdate(il, desc, requestAt120)
		require.NoError(t, err)
		require.Nil(t, result)
	})
}

func TestLoad_CooldownAtChartsLevel(t *testing.T) {
	const cfg = `charts:
- chart:
    name: argo-cd
    repository: https://argoproj.github.io/argo-helm
    version: "*"
  identity: argo-cd
  cooldown_days: 7
`
	ac, err := Load([]byte(cfg))
	require.NoError(t, err)
	require.Len(t, ac.Charts, 1)
	require.NotNil(t, ac.Charts[0].CooldownDays)
	require.Equal(t, 7, *ac.Charts[0].CooldownDays)

	resolved := ac.findUpdateChartForUpdate(&Update{Parse: &LineParse{Identity: "argo-cd"}})
	require.NotNil(t, resolved)
	require.Equal(t, 7, resolved.CooldownDays)
	require.Equal(t, "argo-cd", resolved.Name)
}

func TestResolveCooldownDays_GlobalFallback(t *testing.T) {
	const cfg = `charts:
- chart:
    name: with-override
    repository: https://example.com
    version: "*"
  identity: with-override
  cooldown_days: 3
- chart:
    name: explicit-zero
    repository: https://example.com
    version: "*"
  identity: explicit-zero
  cooldown_days: 0
- chart:
    name: inherits
    repository: https://example.com
    version: "*"
  identity: inherits
cooldown_days: 7
`
	ac, err := Load([]byte(cfg))
	require.NoError(t, err)
	require.NotNil(t, ac.CooldownDays)
	require.Equal(t, 7, *ac.CooldownDays)

	override := ac.findUpdateChartForUpdate(&Update{Parse: &LineParse{Identity: "with-override"}})
	require.Equal(t, 3, override.CooldownDays) // per-chart wins

	zero := ac.findUpdateChartForUpdate(&Update{Parse: &LineParse{Identity: "explicit-zero"}})
	require.Equal(t, 0, zero.CooldownDays) // explicit 0 overrides global, not treated as unset

	inherits := ac.findUpdateChartForUpdate(&Update{Parse: &LineParse{Identity: "inherits"}})
	require.Equal(t, 7, inherits.CooldownDays) // falls back to global
}

func TestApplyUpdatesToFiles_PerChartRegexRestricts(t *testing.T) {
	const cfg = `charts:
- chart:
    name: ` + chartNameTest + `
    repository: https://example.com
    version: "*"
  identity: ` + chartNameTest + `
  filename_regex:
  - clusters/prod/.*\.yaml
filename_regex:
- .*\.yaml
`
	ac, err := Load([]byte(cfg))
	require.NoError(t, err)
	il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-1 * time.Hour))}

	pf := ParseContent("version: 0.0.1 # helm:autoupdate:" + chartNameTest)
	pf.OriginalFilename = "clusters/dev/app.yaml" // matches global, not per-chart

	out, err := ApplyUpdatesToFiles(il, ac, []*ParsedFile{&pf})
	require.NoError(t, err)
	require.Len(t, out, 0)
}

func TestApplyUpdatesToFiles_PerChartOverridesGlobal(t *testing.T) {
	const cfg = `charts:
- chart:
    name: ` + chartNameTest + `
    repository: https://example.com
    version: "*"
  identity: ` + chartNameTest + `
  filename_regex:
  - .*\.txt
filename_regex:
- .*\.yaml
`
	ac, err := Load([]byte(cfg))
	require.NoError(t, err)
	il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-1 * time.Hour))}

	pf := ParseContent("version: 0.0.1 # helm:autoupdate:" + chartNameTest)
	pf.OriginalFilename = "deploy/app.txt" // fails global, matches per-chart

	out, err := ApplyUpdatesToFiles(il, ac, []*ParsedFile{&pf})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Contains(t, string(out[0].Bytes()), "version: 1.0.0 # helm:autoupdate:"+chartNameTest)
}

func TestApplyUpdatesToFiles_FallsBackToGlobal(t *testing.T) {
	const cfg = `charts:
- chart:
    name: ` + chartNameTest + `
    repository: https://example.com
    version: "*"
  identity: ` + chartNameTest + `
filename_regex:
- clusters/.*\.yaml
`
	ac, err := Load([]byte(cfg))
	require.NoError(t, err)
	il := &mockIndexLoader{indexFile: makeIndexFile(time.Now().Add(-1 * time.Hour))}

	match := ParseContent("version: 0.0.1 # helm:autoupdate:" + chartNameTest)
	match.OriginalFilename = "clusters/app.yaml"
	noMatch := ParseContent("version: 0.0.1 # helm:autoupdate:" + chartNameTest)
	noMatch.OriginalFilename = "other/app.yaml"

	out, err := ApplyUpdatesToFiles(il, ac, []*ParsedFile{&match, &noMatch})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "clusters/app.yaml", out[0].OriginalFilename)
}
