package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// A workflow repository is read the way a package archive is: every fetched
// byte is data. Nothing in a checkout is executed, sealed or trusted here; the
// owner still decides trust at project start, and the network is touched only
// inside the explicit search, add and update commands.
const (
	projectWorkflowCatalogVersion    = "prifly-workflow-catalog/1"
	projectWorkflowCatalogFile       = "catalog.yaml"
	projectDefaultWorkflowCatalog    = "https://github.com/StenHigh/prifly-workflows.git"
	projectWorkflowDiscoveryDepth    = 6
	projectWorkflowMaxFiles          = prifly.MaxPackagePayloadFiles
	projectWorkflowMaxFileBytes      = prifly.MaxPackageFileBytes
	projectWorkflowMaxTotalBytes     = prifly.MaxPackageArchiveBytes
	projectWorkflowCatalogMaxBytes   = 1 << 20
	projectWorkflowCatalogMaxEntries = 1000
	projectGitListTimeout            = time.Minute
	projectGitFetchTimeout           = 10 * time.Minute
)

var (
	projectCommitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	projectDigestPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	projectGitHubShorthand   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	projectSCPRepository     = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?[A-Za-z0-9._-]+:[A-Za-z0-9._~/-]+$`)
	projectCredentialPattern = regexp.MustCompile(`://[^/@\s]+@`)
)

// projectWorkflowOrigin is what add or update recorded about a folder. It is
// checked against the local tree digest, never against a signature: a
// statement of provenance, not a trust decision.
type projectWorkflowOrigin struct {
	Repository   string `json:"repository"`
	Path         string `json:"path"`
	Ref          string `json:"ref"`
	Commit       string `json:"commit"`
	Digest       string `json:"digest"`
	ExtendDigest string `json:"extend_digest,omitempty"`
	Catalog      string `json:"catalog,omitempty"`
}

func parseProjectWorkflowOrigin(name string, raw any) (projectWorkflowOrigin, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return projectWorkflowOrigin{}, usageError("project_profile_invalid: package " + name + " origin must be an object")
	}
	origin := projectWorkflowOrigin{}
	fields := map[string]*string{"repository": &origin.Repository, "path": &origin.Path, "ref": &origin.Ref, "commit": &origin.Commit, "digest": &origin.Digest, "extend_digest": &origin.ExtendDigest, "catalog": &origin.Catalog}
	for key, value := range object {
		target, known := fields[key]
		if !known {
			return projectWorkflowOrigin{}, usageError("project_profile_invalid: package " + name + " origin has unknown field " + key)
		}
		text, ok := value.(string)
		if !ok || text == "" {
			return projectWorkflowOrigin{}, usageError("project_profile_invalid: package " + name + " origin " + key + " must be a non-empty string")
		}
		*target = text
	}
	if err := origin.validate(); err != nil {
		return projectWorkflowOrigin{}, usageError("project_profile_invalid: package " + name + " origin " + err.Error())
	}
	return origin, nil
}

func (origin projectWorkflowOrigin) validate() error {
	if repository, err := projectWorkflowRepositoryURL(origin.Repository); err != nil {
		return err
	} else if repository != origin.Repository {
		return errors.New("repository must be the full Git URL")
	}
	if err := projectWorkflowPathValid(origin.Path); err != nil {
		return err
	}
	if err := projectWorkflowRefValid(origin.Ref); err != nil {
		return err
	}
	if !projectCommitPattern.MatchString(origin.Commit) {
		return errors.New("commit must be 40 lowercase hex characters")
	}
	if !projectDigestPattern.MatchString(origin.Digest) || origin.ExtendDigest != "" && !projectDigestPattern.MatchString(origin.ExtendDigest) {
		return errors.New("digest must be sha256:HEX")
	}
	if origin.Catalog != "" {
		if catalog, err := projectWorkflowRepositoryURL(origin.Catalog); err != nil || catalog != origin.Catalog {
			return errors.New("catalog must be the full Git URL")
		}
	}
	return nil
}

// projectWorkflowRepositoryURL admits exactly the transports the git helper
// allows and refuses credentials before any network is used: a token inside a
// tracked project.yaml or a diagnostic would be a leak, not a convenience.
func projectWorkflowRepositoryURL(value string) (string, error) {
	switch {
	case value == "":
		return "", errors.New("repository must not be empty")
	case strings.HasPrefix(value, "-"):
		return "", errors.New("repository must not start with -")
	case strings.ContainsAny(value, " \t\r\n\x00"):
		return "", errors.New("repository must not contain whitespace")
	case strings.Contains(value, "::"):
		return "", errors.New("repository must not use a transport helper such as ext::")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Opaque != "" {
			return "", errors.New("repository is not a valid URL")
		}
		if parsed.User != nil {
			if _, hasPassword := parsed.User.Password(); hasPassword || parsed.Scheme != "ssh" {
				return "", errors.New("repository URL must not carry credentials")
			}
		}
		switch parsed.Scheme {
		case "https", "ssh":
			if parsed.Host == "" {
				return "", errors.New("repository URL requires a host")
			}
		case "file":
			if (parsed.Host != "" && parsed.Host != "localhost") || !strings.HasPrefix(parsed.Path, "/") {
				return "", errors.New("file repository must be an absolute path")
			}
		default:
			return "", errors.New("repository must use https, ssh or file transport")
		}
		return value, nil
	}
	if strings.HasPrefix(value, "/") {
		return value, nil
	}
	if strings.HasPrefix(value, ".") {
		return "", errors.New("repository must be an absolute path or Git URL, not a relative path")
	}
	if projectSCPRepository.MatchString(value) {
		return value, nil
	}
	if projectGitHubShorthand.MatchString(value) {
		owner, repository, _ := strings.Cut(value, "/")
		repository = strings.TrimSuffix(repository, ".git")
		if repository == "" || repository == "." || repository == ".." {
			return "", errors.New("repository must be owner/repo, an absolute path or a Git URL")
		}
		return "https://github.com/" + owner + "/" + repository + ".git", nil
	}
	return "", errors.New("repository must be owner/repo, an absolute path or a Git URL")
}

func projectWorkflowPathValid(value string) error {
	if value == "." {
		return nil
	}
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return errors.New("path must be a clean relative path inside the repository")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." || segment == "." || segment == ".git" {
			return errors.New("path must stay inside the repository and outside .git")
		}
	}
	return nil
}

func projectWorkflowRefValid(value string) error {
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\r\n:?*[\\^~") || strings.Contains(value, "..") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".lock") {
		return errors.New("ref must be a tag, branch or commit name")
	}
	return nil
}

type projectWorkflowSource struct {
	repository   string
	catalogEntry string
}

// parseProjectWorkflowSource reads SOURCE mechanically: a bare launch-style
// name is a catalog entry, owner/repo is GitHub, anything else must already be
// an admitted Git URL or absolute path. Nothing here guesses a host.
func parseProjectWorkflowSource(value string) (projectWorkflowSource, error) {
	if projectLaunchID(value) && !strings.HasPrefix(value, "-") {
		return projectWorkflowSource{catalogEntry: value}, nil
	}
	repository, err := projectWorkflowRepositoryURL(value)
	if err != nil {
		return projectWorkflowSource{}, usageError("project_workflow_source_invalid: " + err.Error())
	}
	return projectWorkflowSource{repository: repository}, nil
}

// projectGit runs one typed argv without a shell, with only the environment a
// credential helper or SSH agent needs, and refuses interactive prompts and
// transport helpers. Fetched text never becomes an argument.
func projectGit(ctx context.Context, directory string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// The user's own template or global hooks path must not run on a checkout
	// of foreign bytes either: hooks are disabled for every invocation here.
	command := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null"}, args...)...)
	command.Dir = directory
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_ALLOW_PROTOCOL=https:ssh:file"}
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		command.Env = append(command.Env, "SSH_AUTH_SOCK="+socket)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("git %s timed out after %s", args[0], timeout)
		}
		message := projectRedactCredentials(strings.TrimSpace(stderr.String()))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func projectRedactCredentials(text string) string {
	return projectCredentialPattern.ReplaceAllString(text, "://***@")
}

type projectWorkflowCheckout struct {
	root   string
	commit string
	ref    string
}

// fetchProjectWorkflowRepository takes one shallow fetch of exactly the
// requested ref into a temporary directory outside the repository. Hooks are
// not transferred by fetch, submodules stay uninitialised and repository
// config is never read, so the checkout is a tree of bytes and nothing more.
func fetchProjectWorkflowRepository(ctx context.Context, repository, ref string) (projectWorkflowCheckout, func(), error) {
	directory, err := os.MkdirTemp("", "prifly-workflow-")
	if err != nil {
		return projectWorkflowCheckout{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	checkout, err := fetchProjectWorkflowInto(ctx, directory, repository, ref)
	if err != nil {
		cleanup()
		return projectWorkflowCheckout{}, nil, err
	}
	return checkout, cleanup, nil
}

func fetchProjectWorkflowInto(ctx context.Context, directory, repository, ref string) (projectWorkflowCheckout, error) {
	unreachable := func(err error) error {
		return usageError("project_workflow_repository_unreachable: " + err.Error())
	}
	if _, err := projectGit(ctx, directory, projectGitListTimeout, "init", "-q", "--template="); err != nil {
		return projectWorkflowCheckout{}, unreachable(err)
	}
	requested := ref
	if requested == "" {
		requested = "HEAD"
	}
	if _, err := projectGit(ctx, directory, projectGitFetchTimeout, "fetch", "-q", "--depth", "1", "--no-tags", "--", repository, requested); err != nil {
		return projectWorkflowCheckout{}, unreachable(err)
	}
	commit, err := projectGit(ctx, directory, projectGitListTimeout, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil || !projectCommitPattern.MatchString(commit) {
		return projectWorkflowCheckout{}, unreachable(errors.New("fetched ref does not name a commit"))
	}
	if _, err := projectGit(ctx, directory, projectGitFetchTimeout, "checkout", "-q", "--detach", commit); err != nil {
		return projectWorkflowCheckout{}, unreachable(err)
	}
	checkout := projectWorkflowCheckout{root: directory, commit: commit, ref: ref}
	if ref == "" {
		checkout.ref = "HEAD"
		if branch := projectRemoteDefaultBranch(ctx, directory, repository); branch != "" {
			checkout.ref = branch
		}
	}
	return checkout, nil
}

func projectRemoteDefaultBranch(ctx context.Context, directory, repository string) string {
	output, err := projectGit(ctx, directory, projectGitListTimeout, "ls-remote", "--symref", "--", repository, "HEAD")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(output, "\n") {
		if target, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			if name, _, _ := strings.Cut(target, "\t"); name != "" {
				return name
			}
		}
	}
	return ""
}

// projectRemoteCommit resolves ref on the remote without fetching. The second
// pattern asks for the peeled form of an annotated tag, so the answer is the
// commit a checkout produces rather than the tag object.
func projectRemoteCommit(ctx context.Context, repository, ref string) (string, error) {
	if projectCommitPattern.MatchString(ref) {
		return ref, nil
	}
	output, err := projectGit(ctx, "", projectGitListTimeout, "ls-remote", "--", repository, ref, ref+"^{}")
	if err != nil {
		return "", usageError("project_workflow_repository_unreachable: " + err.Error())
	}
	commit := ""
	for _, line := range strings.Split(output, "\n") {
		sha, name, ok := strings.Cut(line, "\t")
		if !ok || !projectCommitPattern.MatchString(sha) {
			continue
		}
		if strings.HasSuffix(name, "^{}") {
			return sha, nil
		}
		if commit == "" {
			commit = sha
		}
	}
	if commit == "" {
		return "", usageError("project_workflow_repository_unreachable: " + ref + " is not on the remote")
	}
	return commit, nil
}

func projectWorkflowFolderMarker(workflowPath string) bool {
	info, err := os.Lstat(workflowPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := readFile(workflowPath, flow.MaxDocumentBytes)
	if err != nil {
		return false
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return false
	}
	object, ok := value.(map[string]any)
	return ok && object["authoring"] == projectWorkflowFolderVersion
}

// discoverProjectWorkflowFolders finds folders by their marker alone, never by
// a registration file: a repository declares a workflow by containing one.
func discoverProjectWorkflowFolders(root string) ([]string, error) {
	found := []string{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && relative != "." {
			return fs.SkipDir
		}
		if relative != "." && strings.Count(relative, string(filepath.Separator)) >= projectWorkflowDiscoveryDepth {
			return fs.SkipDir
		}
		if projectWorkflowFolderMarker(filepath.Join(current, "workflow.yaml")) {
			found = append(found, filepath.ToSlash(relative))
			return fs.SkipDir
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

func selectProjectWorkflowFolder(root, requested string) (string, error) {
	if requested != "" {
		target := filepath.Join(root, filepath.FromSlash(requested))
		info, err := os.Lstat(target)
		if err != nil || !info.IsDir() || !projectWorkflowFolderMarker(filepath.Join(target, "workflow.yaml")) {
			return "", usageError("project_workflow_folder_invalid: --path " + requested + " does not name a " + projectWorkflowFolderVersion + " folder")
		}
		return requested, nil
	}
	folders, err := discoverProjectWorkflowFolders(root)
	if err != nil {
		return "", err
	}
	switch len(folders) {
	case 0:
		return "", usageError("project_workflow_repository_empty: no workflow.yaml with authoring " + projectWorkflowFolderVersion + " was found")
	case 1:
		return folders[0], nil
	}
	return "", usageError("project_workflow_repository_ambiguous: choose --path from " + strings.Join(folders, ", "))
}

// projectWorkflowTreeCheck refuses symlinks and submodules from the index
// before anything is copied: a checkout of an uninitialised submodule looks
// like an innocent empty directory on disk.
func projectWorkflowTreeCheck(ctx context.Context, checkout projectWorkflowCheckout, folder string) error {
	output, err := projectGit(ctx, checkout.root, projectGitListTimeout, "ls-files", "--stage", "--", folder)
	if err != nil {
		return usageError("project_workflow_repository_unreachable: " + err.Error())
	}
	for _, line := range strings.Split(output, "\n") {
		mode, rest, _ := strings.Cut(line, " ")
		_, name, _ := strings.Cut(rest, "\t")
		switch mode {
		case "120000":
			return usageError("project_workflow_folder_invalid: symlinks are not allowed: " + name)
		case "160000":
			return usageError("project_workflow_folder_invalid: submodules are not allowed: " + name)
		}
	}
	return nil
}

type projectWorkflowStaging struct {
	root   string
	folder string
}

// stageProjectWorkflowFolder copies regular files into a private root that
// mirrors the final .prifly/workflows/NAME path, so folder-relative sources
// such as decision catalogs validate exactly as they will after the rename.
func stageProjectWorkflowFolder(root, source, name string) (projectWorkflowStaging, error) {
	workflows := filepath.Join(root, ".prifly", "workflows")
	if err := os.MkdirAll(workflows, 0755); err != nil {
		return projectWorkflowStaging{}, err
	}
	stagingRoot, err := os.MkdirTemp(workflows, ".staging-")
	if err != nil {
		return projectWorkflowStaging{}, err
	}
	staging := projectWorkflowStaging{root: stagingRoot, folder: filepath.Join(stagingRoot, ".prifly", "workflows", name)}
	if err := copyProjectWorkflowFolder(source, staging.folder); err != nil {
		staging.discard()
		return projectWorkflowStaging{}, err
	}
	return staging, nil
}

func (staging projectWorkflowStaging) discard() { _ = os.RemoveAll(staging.root) }

func copyProjectWorkflowFolder(source, target string) error {
	files, total := 0, int64(0)
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		destination := filepath.Join(target, relative)
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			return usageError("project_workflow_folder_invalid: symlinks are not allowed: " + name)
		case entry.IsDir():
			if entry.Name() == ".git" {
				return usageError("project_workflow_folder_invalid: nested Git repository is not allowed: " + name)
			}
			return os.MkdirAll(destination, 0755)
		case !entry.Type().IsRegular():
			return usageError("project_workflow_folder_invalid: unsupported file type: " + name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		total += info.Size()
		if files > projectWorkflowMaxFiles || info.Size() > projectWorkflowMaxFileBytes || total > projectWorkflowMaxTotalBytes {
			return usageError("project_workflow_limit: workflow folder exceeds the file count or size limits")
		}
		data, err := readFile(current, projectWorkflowMaxFileBytes)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0644)
	})
}

type projectWorkflowInspection struct {
	source projectPackageSource
	title  string
}

func inspectProjectWorkflowFolder(root, folder string) (projectWorkflowInspection, error) {
	source, err := readProjectWorkflowFolder(root, folder)
	if err != nil {
		return projectWorkflowInspection{}, err
	}
	value, err := projectYAMLDocument(filepath.Join(folder, "workflow.yaml"))
	if err != nil {
		return projectWorkflowInspection{}, usageError("project_workflow_folder_invalid: " + err.Error())
	}
	workflow, err := projectFolderWorkflowDefinition(value)
	if err != nil {
		return projectWorkflowInspection{}, usageError("project_workflow_folder_invalid: " + err.Error())
	}
	title, _ := workflow["title"].(string)
	return projectWorkflowInspection{source: source, title: title}, nil
}

type projectWorkflowPackageView struct {
	ID                   string            `json:"id"`
	Version              string            `json:"version"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	RequiresCoreProtocol string            `json:"requires_core_protocol"`
	References           map[string]string `json:"references"`
}

func (inspection projectWorkflowInspection) view() projectWorkflowPackageView {
	references := inspection.source.References
	if references == nil {
		references = map[string]string{}
	}
	return projectWorkflowPackageView{ID: inspection.source.ID, Version: inspection.source.Version, Title: inspection.title, Description: inspection.source.Description, RequiresCoreProtocol: inspection.source.RequiresCoreProtocol, References: references}
}

// projectWorkflowTreeDigest hashes every file in path order and leaves the root
// extend.yaml out: that file is the team's own configuration, so editing it is
// not drift from the upstream tree.
func projectWorkflowTreeDigest(folder string) (string, error) {
	digests, err := projectWorkflowFileDigests(folder)
	if err != nil {
		return "", err
	}
	paths := make([]string, 0, len(digests))
	for name := range digests {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, name := range paths {
		fmt.Fprintf(hash, "%s\x00%s\n", name, digests[name])
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func projectWorkflowFileDigests(folder string) (map[string]string, error) {
	digests := map[string]string{}
	err := filepath.WalkDir(folder, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return usageError("project_workflow_folder_invalid: symlinks are not allowed")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(folder, current)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "extend.yaml" {
			return nil
		}
		digest, err := projectFileDigest(current)
		if err != nil {
			return err
		}
		digests[name] = digest
		return nil
	})
	return digests, err
}

func projectFileDigest(filePath string) (string, error) {
	data, err := readFile(filePath, projectWorkflowMaxFileBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func projectWorkflowFolderSource(name string) string { return ".prifly/workflows/" + name }

func projectWorkflowFolderPath(root, name string) string {
	return filepath.Join(root, ".prifly", "workflows", name)
}

func readProjectProfileNode(root string) (*yaml.Node, error) {
	data, err := readFile(filepath.Join(root, ".prifly", "project.yaml"), flow.MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, usageError("project_profile_invalid: " + err.Error())
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, usageError("project_profile_invalid: profile must be an object")
	}
	return &document, nil
}

func projectProfileSection(document *yaml.Node, key string) (*yaml.Node, error) {
	section := projectMappingValue(document.Content[0], key)
	if section == nil || section.Kind != yaml.MappingNode {
		return nil, usageError("project_profile_invalid: " + key + " must be an object")
	}
	return section, nil
}

func projectMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func projectMappingSet(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, projectScalarNode(key), value)
	// An empty init-template mapping is flow style ({}); a populated one reads
	// better as a block, and the encoder keeps every other node untouched.
	mapping.Style = 0
}

func projectMappingDelete(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func projectScalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func projectMappingNode(pairs ...string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(pairs); i += 2 {
		node.Content = append(node.Content, projectScalarNode(pairs[i]), projectScalarNode(pairs[i+1]))
	}
	return node
}

func projectOriginNode(origin projectWorkflowOrigin) *yaml.Node {
	node := projectMappingNode("repository", origin.Repository, "path", origin.Path, "ref", origin.Ref, "commit", origin.Commit, "digest", origin.Digest)
	if origin.ExtendDigest != "" {
		projectMappingSet(node, "extend_digest", projectScalarNode(origin.ExtendDigest))
	}
	if origin.Catalog != "" {
		projectMappingSet(node, "catalog", projectScalarNode(origin.Catalog))
	}
	return node
}

func writeProjectProfileNode(root string, document *yaml.Node) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return replaceProjectRunner(filepath.Join(root, ".prifly", "project.yaml"), buffer.String())
}

func registerProjectWorkflow(root, name string, origin projectWorkflowOrigin, launch projectLaunch) error {
	document, err := readProjectProfileNode(root)
	if err != nil {
		return err
	}
	packages, err := projectProfileSection(document, "packages")
	if err != nil {
		return err
	}
	launches, err := projectProfileSection(document, "launches")
	if err != nil {
		return err
	}
	entry := projectMappingNode("source", projectWorkflowFolderSource(name))
	projectMappingSet(entry, "origin", projectOriginNode(origin))
	projectMappingSet(packages, name, entry)
	projectMappingSet(launches, name, projectMappingNode("title", launch.Title, "description", launch.Description, "kind", launch.Kind, "workflow", launch.Workflow))
	return writeProjectProfileNode(root, document)
}

func updateProjectWorkflowOrigin(root, name string, origin projectWorkflowOrigin) error {
	document, err := readProjectProfileNode(root)
	if err != nil {
		return err
	}
	packages, err := projectProfileSection(document, "packages")
	if err != nil {
		return err
	}
	entry := projectMappingValue(packages, name)
	if entry == nil || entry.Kind != yaml.MappingNode {
		return usageError("project_profile_invalid: package " + name + " must be an object")
	}
	projectMappingSet(entry, "origin", projectOriginNode(origin))
	return writeProjectProfileNode(root, document)
}

func unregisterProjectWorkflow(root, name string, launchIDs []string) error {
	document, err := readProjectProfileNode(root)
	if err != nil {
		return err
	}
	packages, err := projectProfileSection(document, "packages")
	if err != nil {
		return err
	}
	launches, err := projectProfileSection(document, "launches")
	if err != nil {
		return err
	}
	projectMappingDelete(packages, name)
	for _, id := range launchIDs {
		projectMappingDelete(launches, id)
	}
	return writeProjectProfileNode(root, document)
}

func projectDeclaredPackageIDs(root string, profile projectProfile) (map[string]string, error) {
	ids := map[string]string{}
	for name, pkg := range profile.Packages {
		folder, err := projectPackageSourceLocation(root, pkg.Source)
		if err != nil {
			return nil, err
		}
		value, err := projectYAMLDocument(filepath.Join(folder, "workflow.yaml"))
		if err != nil {
			return nil, usageError("project_profile_invalid: package " + name + ": " + err.Error())
		}
		object, _ := value.(map[string]any)
		packageValue, _ := object["package"].(map[string]any)
		if id, _ := packageValue["id"].(string); id != "" {
			ids[id] = name
		}
	}
	return ids, nil
}

func projectWorkflowNameAvailable(root string, profile projectProfile, name string) error {
	if _, exists := profile.Packages[name]; exists {
		return usageError("project_workflow_exists: package " + name + " is already declared")
	}
	if _, exists := profile.Launches[name]; exists {
		return usageError("project_workflow_exists: launch " + name + " is already declared")
	}
	if _, err := os.Lstat(projectWorkflowFolderPath(root, name)); err == nil {
		return usageError("project_workflow_exists: " + projectWorkflowFolderSource(name) + " already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func projectWorkflowPositional(command string, args []string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, usageError("project workflows " + command + " requires its positional argument before any flags")
	}
	return args[0], args[1:], nil
}

func (c *cli) projectWorkflowsCommand(ctx context.Context, args []string) error {
	switch args[0] {
	case "search":
		return c.projectWorkflowsSearch(ctx, args[1:])
	case "add":
		return c.projectWorkflowsAdd(ctx, args[1:])
	case "update":
		return c.projectWorkflowsUpdate(ctx, args[1:])
	case "remove":
		return c.projectWorkflowsRemove(ctx, args[1:])
	}
	return usageError("project workflows accepts search, add, update or remove; without a subcommand it lists declared launches")
}

type projectWorkflowInstall struct {
	repository   string
	ref          string
	path         string
	name         string
	pinnedCommit string
	catalog      string
}

type projectWorkflowAddResult struct {
	SchemaVersion string                     `json:"schema_version"`
	Repository    string                     `json:"repository"`
	Name          string                     `json:"name"`
	Folder        string                     `json:"folder"`
	Package       projectWorkflowPackageView `json:"package"`
	Origin        projectWorkflowOrigin      `json:"origin"`
	Launch        string                     `json:"launch"`
	Next          []string                   `json:"next"`
}

func (c *cli) projectWorkflowsAdd(ctx context.Context, args []string) error {
	source, rest, err := projectWorkflowPositional("add", args)
	if err != nil {
		return err
	}
	f := flags("project workflows add")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	ref := f.String("ref", "", "tag, branch or commit; the remote default branch when omitted")
	subdir := f.String("path", "", "workflow folder inside the source repository")
	name := f.String("name", "", "folder name under .prifly/workflows; the source folder name when omitted")
	catalog := f.String("catalog", projectDefaultWorkflowCatalog, "catalog repository that resolves a bare entry name")
	if err := parse(f, rest); err != nil {
		return err
	}
	parsed, err := parseProjectWorkflowSource(source)
	if err != nil {
		return err
	}
	if *ref != "" {
		if err := projectWorkflowRefValid(*ref); err != nil {
			return usageError("project_workflow_source_invalid: " + err.Error())
		}
	}
	if *subdir != "" {
		if err := projectWorkflowPathValid(*subdir); err != nil {
			return usageError("project_workflow_source_invalid: " + err.Error())
		}
	}
	if *name != "" && !projectLaunchID(*name) {
		return usageError("project_workflow_source_invalid: --name must contain lowercase letters, digits, - or _")
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	request := projectWorkflowInstall{repository: parsed.repository, ref: *ref, path: *subdir, name: *name}
	if parsed.catalogEntry != "" {
		if *subdir != "" {
			return usageError("project_workflow_source_invalid: a catalog entry already names its path; --path applies to repositories only")
		}
		catalogURL, err := projectWorkflowRepositoryURL(*catalog)
		if err != nil {
			return usageError("project_workflow_source_invalid: catalog " + err.Error())
		}
		entry, err := lookupProjectWorkflowCatalogEntry(ctx, catalogURL, parsed.catalogEntry)
		if err != nil {
			return err
		}
		request.repository, request.path, request.catalog = entry.Repository, entry.Path, catalogURL
		if request.ref == "" {
			// The pin belongs to the catalog's own ref; an explicit --ref is the
			// user's choice and is not checked against it.
			request.ref, request.pinnedCommit = entry.Ref, entry.Commit
		}
		if request.name == "" {
			request.name = entry.Name
		}
	}
	result, err := installProjectWorkflow(ctx, root, profile, request)
	if err != nil {
		return err
	}
	return c.emit(result)
}

func installProjectWorkflow(ctx context.Context, root string, profile projectProfile, request projectWorkflowInstall) (projectWorkflowAddResult, error) {
	checkout, cleanup, err := fetchProjectWorkflowRepository(ctx, request.repository, request.ref)
	if err != nil {
		return projectWorkflowAddResult{}, err
	}
	defer cleanup()
	if request.pinnedCommit != "" && checkout.commit != request.pinnedCommit {
		return projectWorkflowAddResult{}, usageError("project_workflow_commit_mismatch: the catalog pins " + request.pinnedCommit + " but " + checkout.ref + " resolved to " + checkout.commit)
	}
	folder, err := selectProjectWorkflowFolder(checkout.root, request.path)
	if err != nil {
		return projectWorkflowAddResult{}, err
	}
	name := request.name
	if name == "" {
		name = path.Base(folder)
		if folder == "." {
			name = strings.TrimSuffix(path.Base(request.repository), ".git")
		}
		if !projectLaunchID(name) {
			return projectWorkflowAddResult{}, usageError("project_workflow_source_invalid: " + name + " is not a valid folder name; choose --name")
		}
	}
	if err := projectWorkflowNameAvailable(root, profile, name); err != nil {
		return projectWorkflowAddResult{}, err
	}
	if err := projectWorkflowTreeCheck(ctx, checkout, folder); err != nil {
		return projectWorkflowAddResult{}, err
	}
	staging, err := stageProjectWorkflowFolder(root, filepath.Join(checkout.root, filepath.FromSlash(folder)), name)
	if err != nil {
		return projectWorkflowAddResult{}, err
	}
	inspection, err := inspectProjectWorkflowFolder(staging.root, staging.folder)
	if err != nil {
		staging.discard()
		return projectWorkflowAddResult{}, err
	}
	ids, err := projectDeclaredPackageIDs(root, profile)
	if err != nil {
		staging.discard()
		return projectWorkflowAddResult{}, err
	}
	if other, exists := ids[inspection.source.ID]; exists {
		staging.discard()
		return projectWorkflowAddResult{}, usageError("project_workflow_package_conflict: package " + inspection.source.ID + " is already declared by " + other)
	}
	origin, err := projectWorkflowOriginFor(staging.folder, request, checkout, folder)
	if err != nil {
		staging.discard()
		return projectWorkflowAddResult{}, err
	}
	final := projectWorkflowFolderPath(root, name)
	if err := os.Rename(staging.folder, final); err != nil {
		staging.discard()
		if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTEMPTY) {
			return projectWorkflowAddResult{}, usageError("project_workflow_exists: " + projectWorkflowFolderSource(name) + " already exists")
		}
		return projectWorkflowAddResult{}, err
	}
	staging.discard()
	launch := projectLaunch{Title: inspection.title, Description: inspection.source.Description, Kind: "workflow", Workflow: projectWorkflowFolderSource(name) + "/workflow.yaml"}
	if launch.Title == "" {
		launch.Title = name
	}
	if err := registerProjectWorkflow(root, name, origin, launch); err != nil {
		_ = os.RemoveAll(final)
		return projectWorkflowAddResult{}, err
	}
	return projectWorkflowAddResult{SchemaVersion: "project-workflow-add/1", Repository: root, Name: name, Folder: projectWorkflowFolderSource(name), Package: inspection.view(), Origin: origin, Launch: name, Next: projectWorkflowNextSteps(name)}, nil
}

func projectWorkflowOriginFor(folder string, request projectWorkflowInstall, checkout projectWorkflowCheckout, sourcePath string) (projectWorkflowOrigin, error) {
	digest, err := projectWorkflowTreeDigest(folder)
	if err != nil {
		return projectWorkflowOrigin{}, err
	}
	origin := projectWorkflowOrigin{Repository: request.repository, Path: sourcePath, Ref: checkout.ref, Commit: checkout.commit, Digest: digest, Catalog: request.catalog}
	origin.ExtendDigest, err = projectOptionalFileDigest(filepath.Join(folder, "extend.yaml"))
	if err != nil {
		return projectWorkflowOrigin{}, err
	}
	return origin, nil
}

func projectOptionalFileDigest(filePath string) (string, error) {
	digest, err := projectFileDigest(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return digest, err
}

func projectWorkflowNextSteps(name string) []string {
	return []string{
		"Review " + projectWorkflowFolderSource(name) + " and commit the .prifly changes; nothing was sealed, imported or executed.",
		"Run prifly project compile --repository DIR --package " + name + " --host HOST --output DIR to check it against this repository's host skills.",
		"Trust is decided only when project start seals and imports the package.",
	}
}

type projectCatalogCategory struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type projectCatalogEntry struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Repository  string   `json:"repository"`
	Path        string   `json:"path"`
	Ref         string   `json:"ref,omitempty"`
	Commit      string   `json:"commit,omitempty"`
	Tags        []string `json:"tags"`
}

func (entry projectCatalogEntry) matches(needle string) bool {
	for _, text := range append([]string{entry.Name, entry.Title, entry.Description}, entry.Tags...) {
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return false
}

type projectWorkflowCatalog struct {
	Title      string
	Categories []projectCatalogCategory
	Workflows  []projectCatalogEntry
}

// parseProjectWorkflowCatalog is as strict as every other authoring parser: an
// unknown field is an error, not a hint, because a pointer that is misread
// sends bytes from the wrong place into a project.
func parseProjectWorkflowCatalog(data []byte) (projectWorkflowCatalog, error) {
	invalid := func(message string) error { return usageError("project_workflow_catalog_invalid: " + message) }
	if len(data) > projectWorkflowCatalogMaxBytes {
		return projectWorkflowCatalog{}, invalid(projectWorkflowCatalogFile + " exceeds 1 MiB")
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return projectWorkflowCatalog{}, invalid(err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok {
		return projectWorkflowCatalog{}, invalid(projectWorkflowCatalogFile + " must be an object")
	}
	for key := range object {
		switch key {
		case "schema_version", "title", "categories", "workflows":
		default:
			return projectWorkflowCatalog{}, invalid("unknown field " + key)
		}
	}
	if object["schema_version"] != projectWorkflowCatalogVersion {
		return projectWorkflowCatalog{}, invalid("schema_version must be " + projectWorkflowCatalogVersion)
	}
	catalog := projectWorkflowCatalog{Categories: []projectCatalogCategory{}, Workflows: []projectCatalogEntry{}}
	if raw, exists := object["title"]; exists {
		title, ok := raw.(string)
		if !ok || title == "" {
			return projectWorkflowCatalog{}, invalid("title must be a non-empty string")
		}
		catalog.Title = title
	}
	categories, ok := object["categories"].(map[string]any)
	if !ok {
		return projectWorkflowCatalog{}, invalid("categories must be an object")
	}
	known := map[string]bool{}
	for id, raw := range categories {
		if !projectLaunchID(id) {
			return projectWorkflowCatalog{}, invalid("category " + id + " must contain lowercase letters, digits, - or _")
		}
		fields, err := projectCatalogStrings(raw, "category "+id, []string{"title"}, []string{"description"})
		if err != nil {
			return projectWorkflowCatalog{}, invalid(err.Error())
		}
		known[id] = true
		catalog.Categories = append(catalog.Categories, projectCatalogCategory{ID: id, Title: fields["title"], Description: fields["description"]})
	}
	workflows, ok := object["workflows"].(map[string]any)
	if !ok {
		return projectWorkflowCatalog{}, invalid("workflows must be an object")
	}
	if len(workflows) > projectWorkflowCatalogMaxEntries {
		return projectWorkflowCatalog{}, invalid("workflows exceeds 1000 entries")
	}
	for name, raw := range workflows {
		if !projectLaunchID(name) {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " must contain lowercase letters, digits, - or _")
		}
		entryObject, ok := raw.(map[string]any)
		if !ok {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " must be an object")
		}
		tags, err := projectCatalogTags(entryObject["tags"])
		if err != nil {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " " + err.Error())
		}
		rest := make(map[string]any, len(entryObject))
		for key, value := range entryObject {
			if key != "tags" {
				rest[key] = value
			}
		}
		fields, err := projectCatalogStrings(rest, "workflow "+name, []string{"title", "description", "category", "repository", "path"}, []string{"ref", "commit"})
		if err != nil {
			return projectWorkflowCatalog{}, invalid(err.Error())
		}
		if !known[fields["category"]] {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " names unknown category " + fields["category"])
		}
		if repository, err := projectWorkflowRepositoryURL(fields["repository"]); err != nil || repository != fields["repository"] {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " repository must be a full Git URL without credentials")
		}
		if err := projectWorkflowPathValid(fields["path"]); err != nil {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " " + err.Error())
		}
		if ref := fields["ref"]; ref != "" {
			if err := projectWorkflowRefValid(ref); err != nil {
				return projectWorkflowCatalog{}, invalid("workflow " + name + " " + err.Error())
			}
		}
		if commit := fields["commit"]; commit != "" && !projectCommitPattern.MatchString(commit) {
			return projectWorkflowCatalog{}, invalid("workflow " + name + " commit must be 40 lowercase hex characters")
		}
		catalog.Workflows = append(catalog.Workflows, projectCatalogEntry{Name: name, Title: fields["title"], Description: fields["description"], Category: fields["category"], Repository: fields["repository"], Path: fields["path"], Ref: fields["ref"], Commit: fields["commit"], Tags: tags})
	}
	sort.Slice(catalog.Categories, func(left, right int) bool { return catalog.Categories[left].ID < catalog.Categories[right].ID })
	sort.Slice(catalog.Workflows, func(left, right int) bool {
		if catalog.Workflows[left].Category != catalog.Workflows[right].Category {
			return catalog.Workflows[left].Category < catalog.Workflows[right].Category
		}
		return catalog.Workflows[left].Name < catalog.Workflows[right].Name
	})
	return catalog, nil
}

func projectCatalogStrings(raw any, label string, required, optional []string) (map[string]string, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New(label + " must be an object")
	}
	allowed := map[string]bool{}
	for _, key := range append(append([]string{}, required...), optional...) {
		allowed[key] = true
	}
	result := map[string]string{}
	for key, value := range object {
		if !allowed[key] {
			return nil, errors.New(label + " has unknown field " + key)
		}
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, errors.New(label + " " + key + " must be a non-empty string")
		}
		result[key] = text
	}
	for _, key := range required {
		if result[key] == "" {
			return nil, errors.New(label + " requires " + key)
		}
	}
	return result, nil
}

func projectCatalogTags(raw any) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("tags must be a list")
	}
	tags := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		tag, ok := item.(string)
		if !ok || tag == "" || seen[tag] {
			return nil, errors.New("tags must be unique non-empty strings")
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags, nil
}

func fetchProjectWorkflowCatalog(ctx context.Context, repository string) (projectWorkflowCatalog, string, error) {
	checkout, cleanup, err := fetchProjectWorkflowRepository(ctx, repository, "")
	if err != nil {
		return projectWorkflowCatalog{}, "", err
	}
	defer cleanup()
	data, err := readFile(filepath.Join(checkout.root, projectWorkflowCatalogFile), projectWorkflowCatalogMaxBytes)
	if errors.Is(err, local.ErrBlobLimit) {
		return projectWorkflowCatalog{}, "", usageError("project_workflow_catalog_invalid: " + projectWorkflowCatalogFile + " exceeds 1 MiB")
	}
	if err != nil {
		return projectWorkflowCatalog{}, "", usageError("project_workflow_catalog_invalid: " + repository + " has no readable " + projectWorkflowCatalogFile + " at its root")
	}
	catalog, err := parseProjectWorkflowCatalog(data)
	if err != nil {
		return projectWorkflowCatalog{}, "", err
	}
	return catalog, checkout.commit, nil
}

func lookupProjectWorkflowCatalogEntry(ctx context.Context, repository, name string) (projectCatalogEntry, error) {
	catalog, _, err := fetchProjectWorkflowCatalog(ctx, repository)
	if err != nil {
		return projectCatalogEntry{}, err
	}
	for _, entry := range catalog.Workflows {
		if entry.Name == name {
			return entry, nil
		}
	}
	return projectCatalogEntry{}, usageError("project_workflow_catalog_entry_unknown: " + name + " is not listed in " + repository)
}

type projectCatalogIdentity struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Title      string `json:"title,omitempty"`
}

type projectWorkflowCatalogResult struct {
	SchemaVersion string                   `json:"schema_version"`
	Catalog       projectCatalogIdentity   `json:"catalog"`
	Categories    []projectCatalogCategory `json:"categories"`
	Workflows     []projectCatalogEntry    `json:"workflows"`
}

func (c *cli) projectWorkflowsSearch(ctx context.Context, args []string) error {
	query := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		query, args = args[0], args[1:]
	}
	f := flags("project workflows search")
	category := f.String("category", "", "only entries of this catalog category")
	catalog := f.String("catalog", projectDefaultWorkflowCatalog, "catalog repository")
	if err := parse(f, args); err != nil {
		return err
	}
	repository, err := projectWorkflowRepositoryURL(*catalog)
	if err != nil {
		return usageError("project_workflow_source_invalid: catalog " + err.Error())
	}
	if *category != "" && !projectLaunchID(*category) {
		return usageError("project_workflow_source_invalid: --category must be a catalog category name")
	}
	parsed, commit, err := fetchProjectWorkflowCatalog(ctx, repository)
	if err != nil {
		return err
	}
	needle := strings.ToLower(query)
	workflows := []projectCatalogEntry{}
	for _, entry := range parsed.Workflows {
		if *category != "" && entry.Category != *category || needle != "" && !entry.matches(needle) {
			continue
		}
		workflows = append(workflows, entry)
	}
	return c.emit(projectWorkflowCatalogResult{SchemaVersion: "project-workflow-catalog/1", Catalog: projectCatalogIdentity{Repository: repository, Commit: commit, Title: parsed.Title}, Categories: parsed.Categories, Workflows: workflows})
}

type projectWorkflowRevision struct {
	Commit string `json:"commit"`
	Digest string `json:"digest"`
}

type projectWorkflowUpdateResult struct {
	SchemaVersion           string                     `json:"schema_version"`
	Repository              string                     `json:"repository"`
	Name                    string                     `json:"name"`
	Folder                  string                     `json:"folder"`
	UpToDate                bool                       `json:"up_to_date"`
	Previous                projectWorkflowRevision    `json:"previous"`
	Current                 projectWorkflowRevision    `json:"current"`
	Origin                  projectWorkflowOrigin      `json:"origin"`
	Package                 projectWorkflowPackageView `json:"package"`
	ExtendUpstreamChanged   bool                       `json:"extend_upstream_changed"`
	PackageVersionUnchanged bool                       `json:"package_version_unchanged"`
	Next                    []string                   `json:"next,omitempty"`
}

func (c *cli) projectWorkflowsUpdate(ctx context.Context, args []string) error {
	name, rest, err := projectWorkflowPositional("update", args)
	if err != nil {
		return err
	}
	f := flags("project workflows update")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	ref := f.String("ref", "", "switch the tracked ref; the recorded ref when omitted")
	if err := parse(f, rest); err != nil {
		return err
	}
	if !projectLaunchID(name) {
		return usageError("project_workflow_not_installed: " + name + " is not a declared package name")
	}
	if *ref != "" {
		if err := projectWorkflowRefValid(*ref); err != nil {
			return usageError("project_workflow_source_invalid: " + err.Error())
		}
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	pkg, exists := profile.Packages[name]
	if !exists {
		return usageError("project_workflow_not_installed: " + name + " is not a declared package")
	}
	if pkg.Origin == nil {
		return usageError("project_workflow_origin_missing: " + name + " was not installed by project workflows add; maintain it by hand")
	}
	origin := *pkg.Origin
	folder, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return err
	}
	installed, err := inspectProjectWorkflowFolder(root, folder)
	if err != nil {
		return err
	}
	digest, err := projectWorkflowTreeDigest(folder)
	if err != nil {
		return err
	}
	if digest != origin.Digest {
		return usageError("project_workflow_modified: local changes in " + strings.Join(projectWorkflowDriftPaths(ctx, origin, folder), ", ") + "; remove and add the folder again, or keep maintaining it by hand")
	}
	targetRef := origin.Ref
	if *ref != "" {
		targetRef = *ref
	}
	remote, err := projectRemoteCommit(ctx, origin.Repository, targetRef)
	if err != nil {
		return err
	}
	previous := projectWorkflowRevision{Commit: origin.Commit, Digest: origin.Digest}
	result := projectWorkflowUpdateResult{SchemaVersion: "project-workflow-update/1", Repository: root, Name: name, Folder: pkg.Source, Previous: previous, Current: previous, Origin: origin, Package: installed.view()}
	if remote == origin.Commit {
		result.UpToDate = true
		return c.emit(result)
	}
	checkout, cleanup, err := fetchProjectWorkflowRepository(ctx, origin.Repository, targetRef)
	if err != nil {
		return err
	}
	defer cleanup()
	if checkout.commit == origin.Commit {
		// The listing named a tag object or moved ref that still peels to the
		// installed commit; nothing on disk changes.
		result.UpToDate = true
		return c.emit(result)
	}
	target := filepath.Join(checkout.root, filepath.FromSlash(origin.Path))
	if info, err := os.Lstat(target); err != nil || !info.IsDir() || !projectWorkflowFolderMarker(filepath.Join(target, "workflow.yaml")) {
		return usageError("project_workflow_folder_invalid: " + origin.Path + " is no longer a workflow folder at " + checkout.commit)
	}
	if err := projectWorkflowTreeCheck(ctx, checkout, origin.Path); err != nil {
		return err
	}
	staging, err := stageProjectWorkflowFolder(root, target, name)
	if err != nil {
		return err
	}
	upstreamExtend, err := projectOptionalFileDigest(filepath.Join(staging.folder, "extend.yaml"))
	if err != nil {
		staging.discard()
		return err
	}
	// The team's extend.yaml is configuration, not upstream content: it moves
	// into the new tree byte for byte, and only its upstream twin is reported.
	if data, err := readFile(filepath.Join(folder, "extend.yaml"), flow.MaxDocumentBytes); err == nil {
		if err := os.WriteFile(filepath.Join(staging.folder, "extend.yaml"), data, 0644); err != nil {
			staging.discard()
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		staging.discard()
		return err
	}
	inspection, err := inspectProjectWorkflowFolder(staging.root, staging.folder)
	if err != nil {
		staging.discard()
		return err
	}
	next := origin
	next.Ref, next.Commit, next.ExtendDigest = targetRef, checkout.commit, upstreamExtend
	if next.Digest, err = projectWorkflowTreeDigest(staging.folder); err != nil {
		staging.discard()
		return err
	}
	if err := swapProjectWorkflowFolder(staging, folder, name); err != nil {
		return err
	}
	if err := updateProjectWorkflowOrigin(root, name, next); err != nil {
		return err
	}
	result.Current = projectWorkflowRevision{Commit: next.Commit, Digest: next.Digest}
	result.Origin, result.Package = next, inspection.view()
	result.ExtendUpstreamChanged = upstreamExtend != origin.ExtendDigest
	result.PackageVersionUnchanged = inspection.source.Version == installed.source.Version && next.Digest != origin.Digest
	if result.PackageVersionUnchanged {
		if profile.SchemaVersion == projectVariantProfileVersion {
			result.Next = []string{"Upstream kept the author package.version. The next compile creates a distinct sealed build; existing packages and Runs remain unchanged."}
		} else {
			result.Next = []string{"Upstream changed bytes without a new package.version: the next project start refuses an existing identity conflict. Migrate the shared profile explicitly to prifly-project-profile/3 to keep both builds, or ask the author to bump the version."}
		}
	}
	return c.emit(result)
}

// swapProjectWorkflowFolder replaces the installed folder with the staged one
// through two renames, so no moment leaves a half-written folder declared in
// project.yaml.
func swapProjectWorkflowFolder(staging projectWorkflowStaging, folder, name string) error {
	// MkdirTemp only reserves a unique name: rename cannot replace even an
	// empty directory on every platform, so the placeholder is removed first.
	// ponytail: a concurrent process could take the name in between; two
	// updates of one folder at once are not a supported workflow.
	previous, err := os.MkdirTemp(filepath.Dir(folder), ".previous-"+name+"-")
	if err != nil {
		staging.discard()
		return err
	}
	if err := os.Remove(previous); err != nil {
		staging.discard()
		return err
	}
	if err := os.Rename(folder, previous); err != nil {
		staging.discard()
		return err
	}
	if err := os.Rename(staging.folder, folder); err != nil {
		_ = os.Rename(previous, folder)
		staging.discard()
		return err
	}
	staging.discard()
	return os.RemoveAll(previous)
}

func projectWorkflowDriftPaths(ctx context.Context, origin projectWorkflowOrigin, folder string) []string {
	installed, err := projectWorkflowFileDigests(folder)
	if err != nil {
		return []string{"(the installed tree could not be read)"}
	}
	checkout, cleanup, err := fetchProjectWorkflowRepository(ctx, origin.Repository, origin.Commit)
	if err != nil {
		return []string{"(commit " + origin.Commit + " could not be fetched to name the paths)"}
	}
	defer cleanup()
	upstream, err := projectWorkflowFileDigests(filepath.Join(checkout.root, filepath.FromSlash(origin.Path)))
	if err != nil {
		return []string{"(commit " + origin.Commit + " no longer holds " + origin.Path + ")"}
	}
	changed := []string{}
	for name, digest := range installed {
		if upstream[name] != digest {
			changed = append(changed, name)
		}
	}
	for name := range upstream {
		if _, exists := installed[name]; !exists {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

type projectWorkflowRemoveResult struct {
	SchemaVersion   string   `json:"schema_version"`
	Repository      string   `json:"repository"`
	Name            string   `json:"name"`
	Folder          string   `json:"folder"`
	RemovedLaunches []string `json:"removed_launches"`
}

func (c *cli) projectWorkflowsRemove(ctx context.Context, args []string) error {
	name, rest, err := projectWorkflowPositional("remove", args)
	if err != nil {
		return err
	}
	f := flags("project workflows remove")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	if err := parse(f, rest); err != nil {
		return err
	}
	if !projectLaunchID(name) {
		return usageError("project_workflow_not_installed: " + name + " is not a declared package name")
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	pkg, exists := profile.Packages[name]
	if !exists {
		return usageError("project_workflow_not_installed: " + name + " is not a declared package")
	}
	if pkg.Source != projectWorkflowFolderSource(name) {
		return usageError("project_workflow_not_installed: remove handles only " + projectWorkflowFolderSource(name) + "; " + name + " points at " + pkg.Source)
	}
	folder, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return err
	}
	launchIDs := []string{}
	for id, launch := range profile.Launches {
		if strings.HasPrefix(launch.Workflow, pkg.Source+"/") {
			launchIDs = append(launchIDs, id)
		}
	}
	sort.Strings(launchIDs)
	if err := unregisterProjectWorkflow(root, name, launchIDs); err != nil {
		return err
	}
	if err := os.RemoveAll(folder); err != nil {
		return err
	}
	return c.emit(projectWorkflowRemoveResult{SchemaVersion: "project-workflow-remove/1", Repository: root, Name: name, Folder: pkg.Source, RemovedLaunches: launchIDs})
}
