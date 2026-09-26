package projects

import (
	"context"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// Labels and milestones. Changes publish label.* and milestone.* events.

func (m *Module) requireProject(ctx context.Context, id int64) error {
	_, err := m.q.GetProject(ctx, id)
	return notFound(err)
}

func (m *Module) listLabels(ctx context.Context, projectID int64) ([]api.Label, error) {
	if err := m.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := m.q.ListLabels(ctx, &projectID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Label, len(rows))
	for i, l := range rows {
		out[i] = toLabel(l)
	}
	return out, nil
}

// labelInProject loads a label that the project may edit: its own or a global one.
func (m *Module) labelInProject(ctx context.Context, projectID, labelID int64) (db.Label, error) {
	l, err := m.q.GetLabel(ctx, labelID)
	if err != nil {
		return l, notFound(err)
	}
	if l.ProjectID != nil && *l.ProjectID != projectID {
		return l, httpx.ErrNotFound
	}
	return l, nil
}

func (m *Module) createLabel(ctx context.Context, projectID int64, in api.CreateLabel) (out api.Label, err error) {
	defer func() { m.d.Audit.Record(ctx, "label.create", in.Name, map[string]any{"projectId": projectID}, err) }()
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return out, httpx.Invalid("标签名称不能为空")
	}
	if err := m.requireProject(ctx, projectID); err != nil {
		return out, err
	}
	pid := &projectID
	if in.Global != nil && *in.Global {
		pid = nil
	}
	l, err := m.q.CreateLabel(ctx, db.CreateLabelParams{ProjectID: pid, Name: name, Color: deref(in.Color)})
	if err != nil {
		return out, err
	}
	out = toLabel(l)
	m.d.Bus.Publish("label.created", out)
	return out, nil
}

func (m *Module) updateLabel(ctx context.Context, projectID, labelID int64, in api.UpdateLabel) (out api.Label, err error) {
	defer func() { m.d.Audit.Record(ctx, "label.update", out.Name, map[string]any{"id": labelID}, err) }()
	l, err := m.labelInProject(ctx, projectID, labelID)
	if err != nil {
		return out, err
	}
	if in.Name != nil {
		if l.Name = strings.TrimSpace(*in.Name); l.Name == "" {
			return out, httpx.Invalid("标签名称不能为空")
		}
	}
	if in.Color != nil {
		l.Color = *in.Color
	}
	l, err = m.q.UpdateLabel(ctx, db.UpdateLabelParams{Name: l.Name, Color: l.Color, ID: labelID})
	if err != nil {
		return out, err
	}
	out = toLabel(l)
	m.d.Bus.Publish("label.updated", out)
	return out, nil
}

func (m *Module) deleteLabel(ctx context.Context, projectID, labelID int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "label.delete", "", map[string]any{"id": labelID}, err) }()
	l, err := m.labelInProject(ctx, projectID, labelID)
	if err != nil {
		return err
	}
	if err := m.q.DeleteLabel(ctx, labelID); err != nil {
		return err
	}
	m.d.Bus.Publish("label.deleted", toLabel(l))
	return nil
}

func (m *Module) listMilestones(ctx context.Context, projectID int64) ([]api.Milestone, error) {
	if err := m.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := m.q.ListMilestones(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Milestone, len(rows))
	for i, ms := range rows {
		out[i] = toMilestone(ms)
	}
	return out, nil
}

func (m *Module) milestoneInProject(ctx context.Context, projectID, id int64) (db.Milestone, error) {
	ms, err := m.q.GetMilestone(ctx, id)
	if err != nil {
		return ms, notFound(err)
	}
	if ms.ProjectID != projectID {
		return ms, httpx.ErrNotFound
	}
	return ms, nil
}

func (m *Module) createMilestone(ctx context.Context, projectID int64, in api.CreateMilestone) (out api.Milestone, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "milestone.create", in.Name, map[string]any{"projectId": projectID}, err)
	}()
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return out, httpx.Invalid("里程碑名称不能为空")
	}
	if err := m.requireProject(ctx, projectID); err != nil {
		return out, err
	}
	ms, err := m.q.CreateMilestone(ctx, db.CreateMilestoneParams{ProjectID: projectID, Name: name, DueDate: fromDate(in.DueDate), CreatedAt: m.now()})
	if err != nil {
		return out, err
	}
	out = toMilestone(ms)
	m.d.Bus.Publish("milestone.created", out)
	return out, nil
}

func (m *Module) updateMilestone(ctx context.Context, projectID, id int64, in api.UpdateMilestone, clearDue bool) (out api.Milestone, err error) {
	defer func() { m.d.Audit.Record(ctx, "milestone.update", out.Name, map[string]any{"id": id}, err) }()
	ms, err := m.milestoneInProject(ctx, projectID, id)
	if err != nil {
		return out, err
	}
	if in.Name != nil {
		if ms.Name = strings.TrimSpace(*in.Name); ms.Name == "" {
			return out, httpx.Invalid("里程碑名称不能为空")
		}
	}
	if clearDue {
		ms.DueDate = nil
	} else if in.DueDate != nil {
		ms.DueDate = fromDate(in.DueDate)
	}
	ms, err = m.q.UpdateMilestone(ctx, db.UpdateMilestoneParams{Name: ms.Name, DueDate: ms.DueDate, ID: id})
	if err != nil {
		return out, err
	}
	out = toMilestone(ms)
	m.d.Bus.Publish("milestone.updated", out)
	return out, nil
}

func (m *Module) deleteMilestone(ctx context.Context, projectID, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "milestone.delete", "", map[string]any{"id": id}, err) }()
	ms, err := m.milestoneInProject(ctx, projectID, id)
	if err != nil {
		return err
	}
	if err := m.q.DeleteMilestone(ctx, id); err != nil {
		return err
	}
	m.d.Bus.Publish("milestone.deleted", toMilestone(ms))
	return nil
}
