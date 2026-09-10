package preparedcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pixel365/agbx/internal/project"
)

const (
	stateHomeEnvironmentVariable = "XDG_STATE_HOME"
	stateDirectoryName           = "agbx"
	projectsDirectoryName        = "projects"
	indexFileName                = "prepared-images.json"
	indexVersion                 = 1
)

type Project struct {
	Providers  map[string]ProviderState `json:"providers"`
	ConfigPath string                   `json:"config_path"`
	Version    int                      `json:"version"`
}

type ProviderState struct {
	Images       map[string]ImageState `json:"images"`
	CurrentImage string                `json:"current_image"`
}

type ImageState struct {
	FirstSelectedAt time.Time `json:"first_selected_at"`
	LastSelectedAt  time.Time `json:"last_selected_at"`
}

type ImageReferences struct {
	Current    map[string]struct{}
	Historical map[string]struct{}
}

func Load(configFile string) (Project, error) {
	indexPath, err := projectIndexPath(configFile)
	if err != nil {
		return Project{}, err
	}
	index, found, err := readProject(indexPath)
	if err != nil {
		return Project{}, err
	}
	if !found {
		return newProject(configFile), nil
	}

	return index, nil
}

func Record(configFile string, providerName string, imageReference string) error {
	index, err := Load(configFile)
	if err != nil {
		return err
	}

	provider := index.Providers[providerName]
	if provider.Images == nil {
		provider.Images = make(map[string]ImageState)
	}

	now := time.Now().UTC()
	image := provider.Images[imageReference]
	if image.FirstSelectedAt.IsZero() {
		image.FirstSelectedAt = now
	}
	image.LastSelectedAt = now
	provider.CurrentImage = imageReference
	provider.Images[imageReference] = image
	index.Providers[providerName] = provider

	return save(configFile, index)
}

func AllImageReferences() (ImageReferences, error) {
	directory, err := projectsDirectory()
	if err != nil {
		return ImageReferences{}, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return newImageReferences(), nil
	}
	if err != nil {
		return ImageReferences{}, fmt.Errorf(
			"read prepared image indexes in %q: %w",
			directory,
			err,
		)
	}

	references := newImageReferences()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		indexPath := filepath.Join(directory, entry.Name(), indexFileName)
		index, found, err := readProject(indexPath)
		if err != nil {
			return ImageReferences{}, err
		}
		if !found {
			continue
		}
		addProjectReferences(references, index)
	}

	return references, nil
}

func newProject(configFile string) Project {
	return Project{
		Version:    indexVersion,
		ConfigPath: configFile,
		Providers:  make(map[string]ProviderState),
	}
}

func save(configFile string, index Project) error {
	directory, err := projectIndexDirectory(configFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create prepared image index directory %q: %w", directory, err)
	}
	// #nosec G302 -- The index directory requires execute permission for its owning user.
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("set prepared image index directory permissions %q: %w", directory, err)
	}

	contents, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode prepared image index: %w", err)
	}
	contents = append(contents, '\n')

	temporary, err := os.CreateTemp(directory, ".prepared-images-*")
	if err != nil {
		return fmt.Errorf("create temporary prepared image index: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		// #nosec G703 -- temporaryPath is returned by os.CreateTemp in agbx's private state directory.
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("set temporary prepared image index permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("write temporary prepared image index: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary prepared image index: %w", err)
	}

	indexPath := filepath.Join(directory, indexFileName)
	if err := os.Rename(temporaryPath, indexPath); err != nil {
		return fmt.Errorf("replace prepared image index %q: %w", indexPath, err)
	}

	return nil
}

func projectIndexPath(configFile string) (string, error) {
	directory, err := projectIndexDirectory(configFile)
	if err != nil {
		return "", err
	}

	return filepath.Join(directory, indexFileName), nil
}

func projectIndexDirectory(configFile string) (string, error) {
	identifier, err := project.ID(configFile)
	if err != nil {
		return "", err
	}

	stateHome, err := stateHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(stateHome, stateDirectoryName, projectsDirectoryName, identifier), nil
}

func projectsDirectory() (string, error) {
	stateHome, err := stateHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(stateHome, stateDirectoryName, projectsDirectoryName), nil
}

func newImageReferences() ImageReferences {
	return ImageReferences{
		Current:    make(map[string]struct{}),
		Historical: make(map[string]struct{}),
	}
}

func addProjectReferences(references ImageReferences, index Project) {
	for _, provider := range index.Providers {
		for imageReference := range provider.Images {
			references.Historical[imageReference] = struct{}{}
		}
		if provider.CurrentImage != "" {
			references.Current[provider.CurrentImage] = struct{}{}
		}
	}
}

func readProject(indexPath string) (Project, bool, error) {
	// #nosec G304 -- The path is created from agbx's private state directory and a project ID.
	contents, err := os.ReadFile(indexPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, fmt.Errorf("read prepared image index %q: %w", indexPath, err)
	}

	var index Project
	if err := json.Unmarshal(contents, &index); err != nil {
		return Project{}, false, fmt.Errorf("parse prepared image index %q: %w", indexPath, err)
	}
	if index.Version != indexVersion {
		return Project{}, false, fmt.Errorf(
			"unsupported prepared image index version %d in %q",
			index.Version,
			indexPath,
		)
	}
	if index.Providers == nil {
		index.Providers = make(map[string]ProviderState)
	}

	return index, true, nil
}

func stateHome() (string, error) {
	if directory := os.Getenv(stateHomeEnvironmentVariable); directory != "" {
		return directory, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}

	return filepath.Join(home, ".local", "state"), nil
}
