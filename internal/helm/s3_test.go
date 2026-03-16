package helm_test

import (
	"os"
	"testing"

	"github.com/bostjanbozic/helm-autoupdate/internal/helm"
	"github.com/stretchr/testify/require"
)

func TestLoadS3(t *testing.T) {
	var l helm.DirectLoader
	s3Repo := os.Getenv("S3_REPO")
	if s3Repo == "" {
		t.Skip("S3_REPO is not set")
	}
	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		t.Skip("S3_REGION is not set")
	}
	chart := &helm.AutoUpdateChart{S3Region: s3Region}
	indexFile, err := l.LoadIndexFile(s3Repo, chart)
	require.NoError(t, err)
	require.NotNil(t, indexFile)
	_, err = indexFile.Get("missing-name", "*")
	require.Error(t, err)
}
