package analysis

import (
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

func TestCollectRegistryTargetsSkipsLocalWorkspacePackages(t *testing.T) {
	input := Input{
		Target: AnalysisTarget{
			DisplayName:     "apps/web",
			ManifestPath:    "apps/web/package.json",
			LockfilePath:    "yarn.lock",
			Kind:            TargetKindWorkspace,
			PackageManager:  "yarn",
			OwningDirectory: "apps/web",
		},
		BaseManifest: manifest(nil, nil, nil),
		HeadManifest: manifest(map[string]string{
			"local-lib": "file:../local-lib",
		}, nil, nil),
		BaseLockfile: &npm.Lockfile{
			Manager:   "yarn",
			Packages:  map[string]npm.LockPackage{},
			Importers: map[string]npm.LockImporter{},
		},
		HeadLockfile: &npm.Lockfile{
			Manager: "yarn",
			Packages: map[string]npm.LockPackage{
				"node_modules/local-lib": {
					Path:           "node_modules/local-lib",
					Name:           "local-lib",
					Version:        "1.0.0",
					Resolved:       "file:../local-lib",
					WorkspaceLocal: true,
					Dependencies:   map[string]string{},
				},
			},
			Importers: map[string]npm.LockImporter{},
		},
	}

	targets := CollectRegistryTargets(input)
	if len(targets) != 0 {
		t.Fatalf("expected no registry targets for local workspace package, got %#v", targets)
	}
}
