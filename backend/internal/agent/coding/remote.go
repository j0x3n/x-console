package coding

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B47: clones of repositories registered from a Git connection live in the
// repository folder, <home>/x-console/repos by default ("reposDir" in the
// coding config).

const (
	cloneTimeout = 30 * time.Minute
	fetchTimeout = 10 * time.Minute
)

func (c Config) reposDir() string {
	if d := expandHome(strings.TrimSpace(c.ReposDir)); d != "" {
		return d
	}
	return expandHome("~/x-console/repos")
}

// dirPart is one segment of CodingEnsureRepoParams.Dir.
var dirPart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,99}$`)

// clonePath checks Dir and joins it to the repository folder.
func (s *Service) clonePath(dir string) (string, error) {
	parts := strings.Split(dir, "/")
	if len(parts) == 0 || len(parts) > 4 {
		return "", rpcutil.BadParams("dir must be 1 to 4 path segments")
	}
	for _, p := range parts {
		if !dirPart.MatchString(p) || strings.Trim(p, ".") == "" {
			return "", rpcutil.BadParams("invalid dir segment %q", p)
		}
	}
	return filepath.Join(append([]string{s.cfg.reposDir()}, parts...)...), nil
}

// authEnv passes the token as an extra HTTP header through git's
// environment config, so it is neither on the command line nor on disk.
func authEnv(a *protocol.CodingGitAuth) []string {
	if a == nil || a.Token == "" {
		return nil
	}
	user := a.Username
	if user == "" {
		user = "x-access-token"
	}
	basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + a.Token))
	return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic}
}

// redact hides the token in a message.
func redact(msg string, a *protocol.CodingGitAuth) string {
	if a == nil || a.Token == "" {
		return msg
	}
	user := a.Username
	if user == "" {
		user = "x-access-token"
	}
	msg = strings.ReplaceAll(msg, base64.StdEncoding.EncodeToString([]byte(user+":"+a.Token)), "***")
	return strings.ReplaceAll(msg, a.Token, "***")
}

// redactErr runs redact over an error's message.
func redactErr(err error, a *protocol.CodingGitAuth) error {
	if err == nil {
		return nil
	}
	return rpcutil.Failed("%s", redact(errText(err), a))
}

// checkCloneURL accepts http(s) URLs without credentials in them.
func checkCloneURL(raw string) error {
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return rpcutil.BadParams("cloneUrl must be an http(s) URL")
	}
	host := strings.SplitN(strings.SplitN(raw, "://", 2)[1], "/", 2)[0]
	if strings.Contains(host, "@") {
		return rpcutil.BadParams("cloneUrl must not contain credentials")
	}
	return nil
}

// EnsureRepo clones the repository, or fetches it when the clone exists.
func (s *Service) EnsureRepo(ctx context.Context, p protocol.CodingEnsureRepoParams) (protocol.CodingRepo, error) {
	path, err := s.clonePath(p.Dir)
	if err != nil {
		return protocol.CodingRepo{}, err
	}
	if err := checkCloneURL(p.CloneURL); err != nil {
		return protocol.CodingRepo{}, err
	}
	env := authEnv(p.Auth)
	if isRepo(path) {
		fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		if _, err := gitEnv(fctx, path, env, "remote", "set-url", "origin", p.CloneURL); err != nil {
			return protocol.CodingRepo{}, redactErr(err, p.Auth)
		}
		if _, err := gitEnv(fctx, path, env, "fetch", "--quiet", "--prune", "origin"); err != nil {
			return protocol.CodingRepo{}, redactErr(err, p.Auth)
		}
		// origin/HEAD follows the default branch when it changes upstream.
		_, _ = gitEnv(fctx, path, env, "remote", "set-head", "origin", "--auto")
		return repoInfo(ctx, path), nil
	}
	if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
		return protocol.CodingRepo{}, rpcutil.Failed("%s exists and is not a git repository", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return protocol.CodingRepo{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	if _, err := gitEnv(cctx, filepath.Dir(path), env, "clone", "--quiet", "--", p.CloneURL, path); err != nil {
		_ = os.RemoveAll(path)
		return protocol.CodingRepo{}, redactErr(err, p.Auth)
	}
	return repoInfo(ctx, path), nil
}

// gitEnv is git with extra environment variables.
func gitEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	out, err := gitRawEnv(ctx, dir, 0, env, args...)
	return strings.TrimSpace(string(out)), err
}
