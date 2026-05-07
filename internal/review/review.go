package review

import (
	"path"
	"sort"
	"strings"

	ghclient "gh-dep-risk/internal/github"
)

type Ecosystem string

const (
	EcosystemUnknown   Ecosystem = ""
	EcosystemCargo     Ecosystem = "cargo"
	EcosystemComposer  Ecosystem = "composer"
	EcosystemGoModules Ecosystem = "go-modules"
	EcosystemMaven     Ecosystem = "maven"
	EcosystemNPM       Ecosystem = "npm"
	EcosystemPip       Ecosystem = "pip"
	EcosystemPNPM      Ecosystem = "pnpm"
	EcosystemPoetry    Ecosystem = "poetry"
	EcosystemRubyGems  Ecosystem = "rubygems"
	EcosystemSwiftPM   Ecosystem = "swiftpm"
	EcosystemYarn      Ecosystem = "yarn"
)

type PackageManager string

const (
	PackageManagerUnknown   PackageManager = ""
	PackageManagerCargo     PackageManager = "cargo"
	PackageManagerComposer  PackageManager = "composer"
	PackageManagerGo        PackageManager = "go"
	PackageManagerMaven     PackageManager = "maven"
	PackageManagerNPM       PackageManager = "npm"
	PackageManagerPip       PackageManager = "pip"
	PackageManagerPyProject PackageManager = "pyproject"
	PackageManagerPNPM      PackageManager = "pnpm"
	PackageManagerPoetry    PackageManager = "poetry"
	PackageManagerBundler   PackageManager = "bundler"
	PackageManagerSwiftPM   PackageManager = "swiftpm"
	PackageManagerYarn      PackageManager = "yarn"
)

type Vulnerability struct {
	Severity string
	GHSAID   string
	Summary  string
	URL      string
}

type Change struct {
	ChangeType      string
	ManifestPath    string
	OwningDirectory string
	Ecosystem       Ecosystem
	PackageManager  PackageManager
	Name            string
	Version         string
	Vulnerabilities []Vulnerability
}

type Target struct {
	Identity        string
	ManifestPath    string
	OwningDirectory string
	DisplayName     string
	Ecosystem       Ecosystem
	PackageManager  PackageManager
}

func NormalizeChanges(raw []ghclient.DependencyReviewChange) []Change {
	changes := make([]Change, 0, len(raw))
	for _, item := range raw {
		ecosystem, manager, ok := NormalizeEcosystem(item.Ecosystem)
		if !ok {
			continue
		}
		manifestPath := NormalizeManifestPath(item.Manifest)
		if manifestPath == "" {
			continue
		}
		vulnerabilities := make([]Vulnerability, 0, len(item.Vulnerabilities))
		for _, vuln := range item.Vulnerabilities {
			vulnerabilities = append(vulnerabilities, Vulnerability{
				Severity: vuln.Severity,
				GHSAID:   vuln.GHSAID,
				Summary:  vuln.Summary,
				URL:      vuln.URL,
			})
		}
		changes = append(changes, Change{
			ChangeType:      strings.ToLower(strings.TrimSpace(item.ChangeType)),
			ManifestPath:    manifestPath,
			OwningDirectory: ManifestDirectory(manifestPath),
			Ecosystem:       ecosystem,
			PackageManager:  manager,
			Name:            strings.TrimSpace(item.Name),
			Version:         strings.TrimSpace(item.Version),
			Vulnerabilities: vulnerabilities,
		})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].ManifestPath == changes[j].ManifestPath {
			if changes[i].Ecosystem == changes[j].Ecosystem {
				if changes[i].Name == changes[j].Name {
					return changes[i].ChangeType < changes[j].ChangeType
				}
				return changes[i].Name < changes[j].Name
			}
			return changes[i].Ecosystem < changes[j].Ecosystem
		}
		return changes[i].ManifestPath < changes[j].ManifestPath
	})
	return changes
}

func TargetsFromChanges(changes []Change) []Target {
	grouped := map[string]Target{}
	for _, change := range changes {
		target := Target{
			Identity:        TargetIdentity(change.ManifestPath, change.Ecosystem, change.PackageManager),
			ManifestPath:    change.ManifestPath,
			OwningDirectory: change.OwningDirectory,
			DisplayName:     TargetDisplayName(change.ManifestPath),
			Ecosystem:       change.Ecosystem,
			PackageManager:  change.PackageManager,
		}
		grouped[target.Identity] = target
	}

	targets := make([]Target, 0, len(grouped))
	for _, target := range grouped {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].ManifestPath == targets[j].ManifestPath {
			if targets[i].PackageManager == targets[j].PackageManager {
				return targets[i].Ecosystem < targets[j].Ecosystem
			}
			return targets[i].PackageManager < targets[j].PackageManager
		}
		return targets[i].ManifestPath < targets[j].ManifestPath
	})
	return targets
}

func ChangesByTarget(changes []Change) map[string][]Change {
	grouped := map[string][]Change{}
	for _, change := range changes {
		key := TargetIdentity(change.ManifestPath, change.Ecosystem, change.PackageManager)
		grouped[key] = append(grouped[key], change)
	}
	for key, batch := range grouped {
		sort.Slice(batch, func(i, j int) bool {
			if batch[i].Name == batch[j].Name {
				return batch[i].ChangeType < batch[j].ChangeType
			}
			return batch[i].Name < batch[j].Name
		})
		grouped[key] = batch
	}
	return grouped
}

func TargetIdentity(manifestPath string, ecosystem Ecosystem, packageManager PackageManager) string {
	return NormalizeManifestPath(manifestPath) + "|" + string(ecosystem) + "|" + string(packageManager)
}

func NormalizeEcosystem(value string) (Ecosystem, PackageManager, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cargo", "rust":
		return EcosystemCargo, PackageManagerCargo, true
	case "composer", "php":
		return EcosystemComposer, PackageManagerComposer, true
	case "go", "gomod", "gomodule", "gomodules", "go mod", "go module", "go modules", "go-module", "go-modules", "go_module", "go_modules", "golang":
		return EcosystemGoModules, PackageManagerGo, true
	case "maven":
		return EcosystemMaven, PackageManagerMaven, true
	case "npm", "node", "javascript":
		return EcosystemNPM, PackageManagerNPM, true
	case "pip", "pypi", "python":
		return EcosystemPip, PackageManagerPip, true
	case "pnpm":
		return EcosystemPNPM, PackageManagerPNPM, true
	case "poetry":
		return EcosystemPoetry, PackageManagerPoetry, true
	case "rubygems", "rubygem", "ruby-gems", "ruby_gems", "ruby", "bundler", "gem", "gems":
		return EcosystemRubyGems, PackageManagerBundler, true
	case "swiftpm", "swift-pm", "swift_pm", "swift package manager", "swift-package-manager", "swift_package_manager", "swift":
		return EcosystemSwiftPM, PackageManagerSwiftPM, true
	case "yarn":
		return EcosystemYarn, PackageManagerYarn, true
	default:
		return EcosystemUnknown, PackageManagerUnknown, false
	}
}

func SupportsDependencyReviewEcosystem(ecosystem Ecosystem) bool {
	switch ecosystem {
	case EcosystemCargo, EcosystemComposer, EcosystemGoModules, EcosystemMaven, EcosystemNPM, EcosystemPip, EcosystemPNPM, EcosystemPoetry, EcosystemRubyGems, EcosystemSwiftPM, EcosystemYarn:
		return true
	default:
		return false
	}
}

func HasLocalFallback(packageManager PackageManager) bool {
	switch packageManager {
	case PackageManagerNPM, PackageManagerPip, PackageManagerPyProject, PackageManagerPNPM, PackageManagerPoetry, PackageManagerYarn:
		return true
	default:
		return false
	}
}

func IsJSEcosystem(ecosystem Ecosystem) bool {
	switch ecosystem {
	case EcosystemNPM, EcosystemPNPM, EcosystemYarn:
		return true
	default:
		return false
	}
}

func NormalizeManifestPath(value string) string {
	cleaned := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	switch cleaned {
	case ".", "/":
		return ""
	default:
		return strings.TrimPrefix(cleaned, "./")
	}
}

func ManifestDirectory(manifestPath string) string {
	cleaned := NormalizeManifestPath(manifestPath)
	if cleaned == "" {
		return ""
	}
	dir := path.Dir(cleaned)
	if dir == "." {
		return ""
	}
	return dir
}

func TargetDisplayName(manifestPath string) string {
	cleaned := NormalizeManifestPath(manifestPath)
	if cleaned == "" {
		return "root"
	}
	if dir := ManifestDirectory(cleaned); dir != "" {
		return dir
	}
	switch cleaned {
	case "package.json", "go.mod", "Cargo.toml", "pom.xml", "composer.json", "requirements.txt", "Gemfile", "Package.swift", "pyproject.toml":
		return "root"
	default:
		return cleaned
	}
}
