package notes

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B32 笔记自动标题和标签（docs/specs/B32.md 的“后端（待做，给开发者）”）

func (m *Module) GetNoteAiSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutNoteAiSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DismissNoteSuggestedTags(w http.ResponseWriter, r *http.Request, noteId api.NoteId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
