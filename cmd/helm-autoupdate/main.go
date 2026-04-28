package main

import (
	"log/slog"
	"os"

	"github.com/bostjanbozic/helm-autoupdate/internal/helm"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	currentDirectory, err := os.Getwd()
	if err != nil {
		slog.Error("unable to get current working directory", "error", err)
		os.Exit(1)
	}
	l := helm.CachedLoader{
		IndexLoader: &helm.DirectLoader{},
	}
	x := helm.DirectorySearchForChanges{
		Dir: currentDirectory,
	}
	ac, err := helm.LoadFile(".helm-autoupdate.yaml")
	if err != nil {
		slog.Error("unable to load .helm-autoupdate.yaml", "error", err)
		os.Exit(1)
	}
	changeFiles, err := x.FindRequestedChanges(ac.ParsedRegex)
	if err != nil {
		slog.Error("unable to find requested changes", "error", err)
		os.Exit(1)
	}
	updatedFiles, err := helm.ApplyUpdatesToFiles(&l, ac, changeFiles)
	if err != nil {
		slog.Error("unable to apply updates to files", "error", err)
		os.Exit(1)
	}
	err = helm.WriteChangesToFilesystem(updatedFiles)
	if err != nil {
		slog.Error("unable to write changes to filesystem", "error", err)
		os.Exit(1)
	}
}
