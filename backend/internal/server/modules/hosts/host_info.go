package hosts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
)

type hostInfoRow struct {
	HostID, Ownership, Client, Username, Password, Note, Tags string
	Order                                                     int64
	Country, CountryIP, Addresses                             string
}

func readHostInfo(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (hostInfoRow, error) {
	v := hostInfoRow{HostID: id, Ownership: "own", Tags: "[]", Addresses: "[]"}
	err := q.QueryRowContext(ctx, `SELECT host_id,ownership,client,username,password_enc,note,tags,sort_order,country_code,country_ip,addresses FROM host_info WHERE host_id=?`, id).Scan(&v.HostID, &v.Ownership, &v.Client, &v.Username, &v.Password, &v.Note, &v.Tags, &v.Order, &v.Country, &v.CountryIP, &v.Addresses)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	return v, err
}

func hostInfoInput(value any) (contracts.HostInfoInput, error) {
	var out contracts.HostInfoInput
	raw, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (m *Module) Validate(ctx context.Context, in contracts.HostInfoInput) error {
	if in.Ownership != nil && *in.Ownership != "own" && *in.Ownership != "client" {
		return httpx.Invalid("归属只能是自己的或客户的")
	}
	for _, v := range []*string{in.Client, in.Username} {
		if v != nil && len(*v) > 255 {
			return httpx.Invalid("名称或用户名过长")
		}
	}
	if in.Note != nil && len(*in.Note) > 65536 {
		return httpx.Invalid("备注过长")
	}
	if in.Password != nil && len(*in.Password) > 4096 {
		return httpx.Invalid("密码过长")
	}
	if in.Password != nil && in.ClearPassword != nil && *in.ClearPassword {
		return httpx.Invalid("设置密码和清除密码不能同时使用")
	}
	if in.Password != nil || (in.ClearPassword != nil && *in.ClearPassword) {
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
	}
	if in.Tags != nil {
		if len(*in.Tags) > 30 {
			return httpx.Invalid("标签最多三十个")
		}
		for _, tag := range *in.Tags {
			if len(strings.TrimSpace(tag)) == 0 || len(tag) > 80 {
				return httpx.Invalid("标签不能为空或过长")
			}
		}
	}
	return nil
}

func (m *Module) ensureHostInfo(ctx context.Context, tx *sql.Tx, id, kind string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO host_info(host_id,sort_order) SELECT ?,COALESCE(max(sort_order),-1)+1 FROM host_info WHERE host_id IN(SELECT id FROM agents WHERE kind=? AND revoked_at IS NULL UNION SELECT 'ssh:'||id FROM ssh_hosts WHERE ?='server') ON CONFLICT(host_id) DO NOTHING`, id, kind, kind)
	return err
}

func (m *Module) writeHostInfo(ctx context.Context, tx *sql.Tx, id, kind string, in contracts.HostInfoInput) error {
	if err := m.ensureHostInfo(ctx, tx, id, kind); err != nil {
		return err
	}
	v, err := readHostInfo(ctx, tx, id)
	if err != nil {
		return err
	}
	if in.Ownership != nil {
		v.Ownership = *in.Ownership
	}
	if in.Client != nil {
		v.Client = strings.TrimSpace(*in.Client)
	}
	if v.Ownership == "own" {
		v.Client = ""
	}
	if in.Username != nil {
		v.Username = *in.Username
	}
	if in.Note != nil {
		v.Note = *in.Note
	}
	if in.Password != nil {
		if *in.Password == "" {
			v.Password = ""
		} else {
			v.Password, err = m.d.Secrets.Seal(*in.Password)
			if err != nil {
				return err
			}
		}
	}
	if in.ClearPassword != nil && *in.ClearPassword {
		v.Password = ""
	}
	if in.Tags != nil {
		tags := []string{}
		seen := map[string]bool{}
		for _, tag := range *in.Tags {
			tag = strings.TrimSpace(tag)
			if !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
			}
		}
		raw, _ := json.Marshal(tags)
		v.Tags = string(raw)
	}
	_, err = tx.ExecContext(ctx, `UPDATE host_info SET ownership=?,client=?,username=?,password_enc=?,note=?,tags=? WHERE host_id=?`, v.Ownership, v.Client, v.Username, v.Password, v.Note, v.Tags, id)
	return err
}

func (m *Module) SavePairing(ctx context.Context, tx *sql.Tx, codeHash string, in contracts.HostInfoInput) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	sealed, err := m.d.Secrets.Seal(string(raw))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO host_pairing_info(code_hash,payload_enc) VALUES(?,?)", codeHash, sealed)
	return err
}

func (m *Module) ApplyPairing(ctx context.Context, tx *sql.Tx, codeHash, id, kind string) error {
	var sealed string
	err := tx.QueryRowContext(ctx, "SELECT payload_enc FROM host_pairing_info WHERE code_hash=?", codeHash).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return m.ensureHostInfo(ctx, tx, id, kind)
	}
	if err != nil {
		return err
	}
	plain, err := m.d.Secrets.Open(sealed)
	if err != nil {
		return err
	}
	var in contracts.HostInfoInput
	if err = json.Unmarshal([]byte(plain), &in); err != nil {
		return err
	}
	if err = m.writeHostInfo(ctx, tx, id, kind, in); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM host_pairing_info WHERE code_hash=?", codeHash)
	return err
}

func (m *Module) PatchHost(w http.ResponseWriter, r *http.Request, id string) {
	ref, err := m.host(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.HostPatch
	if err = httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Name != nil && (strings.TrimSpace(*body.Name) == "" || len(*body.Name) > 255) {
		httpx.Fail(w, r, httpx.Invalid("名称不能为空或过长"))
		return
	}
	in := contracts.HostInfoInput{}
	if body.Info != nil {
		in, err = hostInfoInput(body.Info)
		if err == nil {
			err = m.Validate(r.Context(), in)
		}
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		err = m.writeHostInfo(r.Context(), tx, id, ref.Kind, in)
		if err == nil && body.Name != nil {
			if ref.ssh != nil {
				_, err = tx.ExecContext(r.Context(), "UPDATE ssh_hosts SET name=? WHERE id=?", strings.TrimSpace(*body.Name), ref.ssh.ID)
			} else {
				_, err = tx.ExecContext(r.Context(), "UPDATE agents SET name=? WHERE id=? AND revoked_at IS NULL", strings.TrimSpace(*body.Name), id)
			}
		}
		if err == nil {
			err = tx.Commit()
		}
	}
	if tx != nil {
		_ = tx.Rollback()
	}
	m.d.Audit.Record(r.Context(), "host.info.update", id, map[string]any{"passwordChanged": in.Password != nil || (in.ClearPassword != nil && *in.ClearPassword)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("host.info.updated", map[string]string{"hostId": id})
	out, err := m.detail(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetHostPassword(w http.ResponseWriter, r *http.Request, id string) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.host(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := readHostInfo(r.Context(), m.d.DB, id)
	password := ""
	if err == nil && v.Password != "" {
		password, err = m.d.Secrets.Open(v.Password)
	}
	m.d.Audit.Record(r.Context(), "host.password.read", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]string{"password": password})
}

func (m *Module) PutHostsOrder(w http.ResponseWriter, r *http.Request) {
	var body api.HostOrder
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	kind := string(body.Kind)
	if kind != "server" && kind != "desktop" {
		httpx.Fail(w, r, httpx.Invalid("机器类型无效"))
		return
	}
	if m.kindHidden(r.Context(), kind) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		rows, e := tx.QueryContext(r.Context(), `SELECT id FROM agents WHERE kind=? AND revoked_at IS NULL UNION SELECT 'ssh:'||id FROM ssh_hosts WHERE ?='server'`, kind, kind)
		err = e
		existing := map[string]bool{}
		if err == nil {
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					break
				}
				existing[id] = true
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
		}
		if err == nil && len(existing) != len(body.Ids) {
			err = httpx.Invalid("排序需包含该类型的全部机器")
		}
		seen := map[string]bool{}
		if err == nil {
			for i, id := range body.Ids {
				if !existing[id] || seen[id] {
					err = httpx.Invalid("机器列表含重复或无效项")
					break
				}
				seen[id] = true
				if err = m.ensureHostInfo(r.Context(), tx, id, kind); err != nil {
					break
				}
				if _, err = tx.ExecContext(r.Context(), "UPDATE host_info SET sort_order=? WHERE host_id=?", i, id); err != nil {
					break
				}
			}
		}
		if err == nil {
			err = tx.Commit()
		}
	}
	if tx != nil {
		_ = tx.Rollback()
	}
	m.d.Audit.Record(r.Context(), "host.order.update", kind, map[string]any{"count": len(body.Ids)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("host.order.updated", map[string]string{"kind": kind})
	httpx.NoContent(w)
}

func hostInfoDTO(v hostInfoRow) api.HostInfo {
	out := api.HostInfo{Ownership: api.HostInfoOwnership(v.Ownership), HasPassword: v.Password != "", Tags: []string{}}
	if v.Client != "" {
		out.Client = &v.Client
	}
	if v.Username != "" {
		out.Username = &v.Username
	}
	if v.Note != "" {
		out.Note = &v.Note
	}
	_ = json.Unmarshal([]byte(v.Tags), &out.Tags)
	if out.Tags == nil {
		out.Tags = []string{}
	}
	return out
}

func (m *Module) initializeHostInfo(ctx context.Context) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,kind FROM agents WHERE revoked_at IS NULL UNION ALL SELECT 'ssh:'||id,'server' FROM ssh_hosts ORDER BY kind,id`)
	if err != nil {
		return err
	}
	type host struct{ id, kind string }
	list := []host{}
	for rows.Next() {
		var v host
		if err = rows.Scan(&v.id, &v.kind); err != nil {
			break
		}
		list = append(list, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range list {
		if err = m.ensureHostInfo(ctx, tx, v.id, v.kind); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var _ contracts.HostPairingInfo = (*Module)(nil)
