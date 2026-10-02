package github

import "context"

type remoteRelease struct {
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (m *Module) syncRelease(ctx context.Context, c *restClient, k repoKey) error {
	out := []cacheObject{}
	if c.forge == "forgejo" {
		var batch []remoteRelease
		q := pageQuery(c, 1, 1)
		q.Set("draft", "false")
		q.Set("pre-release", "false")
		if err := c.get(ctx, "/repos/"+k.Repo+"/releases", q, &batch); err != nil {
			return err
		}
		for _, r := range batch {
			if r.Tag != "" && !r.Draft && !r.Prerelease {
				out = append(out, object(r.Tag, r))
				break
			}
		}
	} else {
		var r remoteRelease
		err := c.get(ctx, "/repos/"+k.Repo+"/releases/latest", nil, &r)
		if err != nil && statusOf(err) != 404 {
			return err
		}
		if err == nil && r.Tag != "" {
			out = append(out, object(r.Tag, r))
		}
	}
	return m.saveCompared(ctx, k, "release", out)
}
func (m *Module) saveCompared(ctx context.Context, k repoKey, resource string, objects []cacheObject) error {
	return m.compareObjects(ctx, k, resource, objects)
}
