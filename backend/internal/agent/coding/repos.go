package coding

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Executors detects claude and codex: the program path and its version.
func (s *Service) Executors(ctx context.Context) protocol.CodingExecutorList {
	out := protocol.CodingExecutorList{Items: []protocol.CodingExecutor{}}
	for _, name := range Executors {
		e := protocol.CodingExecutor{Name: name}
		path, err := s.cfg.resolve(name)
		if err != nil {
			e.Error = "not found: " + err.Error()
			out.Items = append(out.Items, e)
			continue
		}
		e.Path = path
		version, err := programVersion(ctx, path)
		if err != nil {
			e.Error = err.Error()
		} else {
			e.Available, e.Version = true, version
		}
		out.Items = append(out.Items, e)
	}
	return out
}

// programVersion runs "<path> --version" and returns its first line.
func programVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(buf.String()), "\n", 2)[0])
	if err != nil {
		if line != "" {
			return "", &gitError{args: []string{"--version"}, msg: line}
		}
		return "", err
	}
	return line, nil
}

// maxRepos caps a scan.
const maxRepos = 500

// skipDirs are never descended into.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "target": true, "dist": true, "build": true, "__pycache__": true}

// Repos scans the roots for git repositories.
func (s *Service) Repos(ctx context.Context, p protocol.CodingReposParams) (protocol.CodingRepoList, error) {
	roots := p.Roots
	if len(roots) == 0 {
		roots = s.cfg.roots()
	} else {
		for i, r := range roots {
			roots[i] = expandHome(strings.TrimSpace(r))
		}
	}
	depth := p.Depth
	if depth == 0 {
		depth = s.cfg.depth()
	}
	var paths []string
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		walk(filepath.Clean(root), depth, func(dir string) bool {
			if len(paths) >= maxRepos {
				return false
			}
			if !seen[dir] {
				seen[dir] = true
				paths = append(paths, dir)
			}
			return true
		})
	}
	sort.Strings(paths)
	out := protocol.CodingRepoList{Items: []protocol.CodingRepo{}, Roots: roots}
	for _, path := range paths {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Items = append(out.Items, repoInfo(ctx, path))
	}
	return out, nil
}

// walk calls found for every repository at most depth levels below dir
// (depth < 0: only dir itself). It does not descend into repositories or
// hidden directories. found returns false to stop.
func walk(dir string, depth int, found func(string) bool) bool {
	if isRepo(dir) {
		return found(dir)
	}
	if depth < 1 {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || skipDirs[name] {
			continue
		}
		if !walk(filepath.Join(dir, name), depth-1, found) {
			return false
		}
	}
	return true
}

// isRepo reports whether dir has a .git directory or file (worktrees and
// submodules use a file).
func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// repoInfo reads branch and remote information. Failures leave fields empty.
func repoInfo(ctx context.Context, path string) protocol.CodingRepo {
	r := protocol.CodingRepo{Path: path, Name: filepath.Base(path)}
	if b, err := git(ctx, path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && b != "HEAD" {
		r.CurrentBranch = b
	}
	r.RemoteURL, _ = git(ctx, path, "remote", "get-url", "origin")
	if ref, err := git(ctx, path, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		r.DefaultBranch = strings.TrimPrefix(ref, "origin/")
	}
	if r.DefaultBranch == "" {
		for _, b := range []string{"main", "master", "trunk", "develop"} {
			if branchExists(ctx, path, b) || remoteBranchExists(ctx, path, b) {
				r.DefaultBranch = b
				break
			}
		}
	}
	if r.DefaultBranch == "" {
		r.DefaultBranch = r.CurrentBranch
	}
	return r
}

// remoteBranchExists reports whether refs/remotes/origin/branch exists.
func remoteBranchExists(ctx context.Context, repo, branch string) bool {
	_, err := git(ctx, repo, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return err == nil
}
