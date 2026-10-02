package drive

import (
	"errors"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

func shareThumbnailType(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (m *Module) GetPublicShareThumbnail(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.GetPublicShareThumbnailParams) {
	share, root, ok := m.publicShare(w, r, token, params.T)
	if !ok {
		return
	}
	id := root.ID
	if params.Item != nil {
		id = *params.Item
	} else if root.IsDir != 0 {
		fail(w, r, errPublicShare)
		return
	}
	item, _, err := m.scopedShareItem(r.Context(), root, id)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.Sha256 == "" || !shareThumbnailType(item.Mime) {
		fail(w, r, errPublicShare)
		return
	}
	if share.MaxDownloads != nil && share.Downloads >= *share.MaxDownloads {
		fail(w, r, errShareLimit)
		return
	}
	stream, info, err := files.OpenSeeker(r.Context(), m.store, thumbnailKey(item.Sha256))
	if errors.Is(err, files.ErrNotFound) {
		err = m.makeThumbnail(r.Context(), item.Sha256)
		if err == nil {
			stream, info, err = files.OpenSeeker(r.Context(), m.store, thumbnailKey(item.Sha256))
		}
	}
	if errors.Is(err, files.ErrNotFound) {
		err = errPublicShare
	}
	if fail(w, r, err) {
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "", info.ModTime, stream)
}
