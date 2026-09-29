package monitoring

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

var imageRef = regexp.MustCompile(`^[a-zA-Z0-9:._/@-]{1,300}$`)

// RemoveImage is DELETE /hosts/{hostId}/docker/images/{imageId}.
func (m *Module) RemoveImage(w http.ResponseWriter, r *http.Request, hostID api.HostId, imageID string) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !imageRef.MatchString(imageID) || strings.Contains(imageID, "..") {
		httpx.Fail(w, r, httpx.Invalid("镜像 ID 里有不能用的字符"))
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := m.dockerCall(cctx, hostID, protocol.MethodDockerImageRemove, protocol.DockerImageRemoveParams{ID: imageID}, nil)
	err = m.imageErr(cctx, hostID, imageID, err)
	m.d.Audit.Record(ctx, "docker.image_remove", hostID, map[string]any{"image": imageID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("docker.image_removed", map[string]any{"hostId": hostID, "id": imageID})
	httpx.NoContent(w)
}

// PruneImages is POST /hosts/{hostId}/docker/images/prune.
func (m *Module) PruneImages(w http.ResponseWriter, r *http.Request, hostID api.HostId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var res protocol.DockerImagePruneResult
	err := m.dockerCall(cctx, hostID, protocol.MethodDockerImagePrune, nil, &res)
	m.d.Audit.Record(ctx, "docker.image_prune", hostID, map[string]any{"deleted": res.Deleted, "spaceReclaimed": res.SpaceReclaimed}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("docker.images_pruned", map[string]any{"hostId": hostID, "deleted": res.Deleted, "spaceReclaimed": res.SpaceReclaimed})
	httpx.JSON(w, http.StatusOK, map[string]any{"deleted": res.Deleted, "spaceReclaimed": res.SpaceReclaimed})
}

// imageErr gives an image error the right words. A 404 on a container
// would say "容器不存在", so it is changed here, and a 409 names the
// containers that use the image.
func (m *Module) imageErr(ctx context.Context, hostID, image string, err error) error {
	var he *httpx.Error
	if err == nil || !errors.As(err, &he) {
		return err
	}
	switch he.Status {
	case http.StatusNotFound:
		return httpx.NewError(http.StatusNotFound, "not_found", "镜像不存在")
	case http.StatusConflict:
		names := m.imageUsers(ctx, hostID, image)
		if len(names) == 0 {
			return httpx.NewError(http.StatusConflict, "conflict", "有容器在用这个镜像")
		}
		return httpx.NewError(http.StatusConflict, "conflict", "有容器在用这个镜像："+strings.Join(names, "、"))
	}
	return err
}

// imageUsers finds the containers (running or not) made from an image.
func (m *Module) imageUsers(ctx context.Context, hostID, image string) []string {
	var images protocol.DockerImageList
	var containers protocol.DockerContainerList
	if m.dockerCall(ctx, hostID, protocol.MethodDockerImages, nil, &images) != nil ||
		m.dockerCall(ctx, hostID, protocol.MethodDockerPS, protocol.DockerPSParams{All: true}, &containers) != nil {
		return nil
	}
	// Names a container can show for the image: its tags, its full ID and the
	// short form of it.
	refs := map[string]bool{image: true}
	short := func(id string) string { return strings.TrimPrefix(id, "sha256:") }
	for _, img := range images.Items {
		if img.ID != image && short(img.ID) != short(image) && !contains(img.Tags, image) {
			continue
		}
		refs[img.ID], refs[short(img.ID)] = true, true
		if len(short(img.ID)) >= 12 {
			refs[short(img.ID)[:12]] = true
		}
		for _, t := range img.Tags {
			refs[t] = true
		}
	}
	var names []string
	for _, c := range containers.Items {
		if refs[c.Image] || refs[short(c.Image)] {
			names = append(names, c.Name)
		}
	}
	return names
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
