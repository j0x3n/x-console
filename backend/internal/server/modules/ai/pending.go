package ai

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B32（docs/specs/B32.md）、B33（docs/specs/B33.md）的“后端（待做，给开发者）”

func (m *Module) CreateAiProvider(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListAiProviders(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UpdateAiProvider(w http.ResponseWriter, r *http.Request, providerId api.ProviderId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DeleteAiProvider(w http.ResponseWriter, r *http.Request, providerId api.ProviderId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) TestAiProvider(w http.ResponseWriter, r *http.Request, providerId api.ProviderId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) RefreshAiModels(w http.ResponseWriter, r *http.Request, providerId api.ProviderId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListAiModels(w http.ResponseWriter, r *http.Request, params api.ListAiModelsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) SetAiModelSpec(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetAiModelSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutAiModelSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetAiUsage(w http.ResponseWriter, r *http.Request, params api.GetAiUsageParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListHostAgentConversations(w http.ResponseWriter, r *http.Request, hostId string) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CreateHostAgentConversation(w http.ResponseWriter, r *http.Request, hostId string) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) SetAiConversationPermission(w http.ResponseWriter, r *http.Request, conversationId api.ConversationId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
