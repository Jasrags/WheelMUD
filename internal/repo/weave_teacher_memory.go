package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryWeaveTeacherRepo is a map-backed WeaveTeacherRepo for tests
// and non-persistent runs. Concurrent-safe.
type MemoryWeaveTeacherRepo struct {
	mu       sync.RWMutex
	nextID   int64
	teachers map[int64]WeaveTeacher
	byMobTpl map[int64]int64 // mob_template_id -> teacher_id
}

func NewMemoryWeaveTeacherRepo() *MemoryWeaveTeacherRepo {
	return &MemoryWeaveTeacherRepo{
		teachers: make(map[int64]WeaveTeacher),
		byMobTpl: make(map[int64]int64),
	}
}

func (r *MemoryWeaveTeacherRepo) Create(_ context.Context, t WeaveTeacher) (WeaveTeacher, error) {
	if t.MobTemplateID == 0 {
		return WeaveTeacher{}, ErrInvalidExternalID
	}
	if t.MaxLevelTaught < 0 || t.MaxLevelTaught > 9 {
		return WeaveTeacher{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byMobTpl[t.MobTemplateID]; dup {
		return WeaveTeacher{}, ErrDuplicateExternalID
	}
	r.nextID++
	t.ID = r.nextID
	r.teachers[t.ID] = t
	r.byMobTpl[t.MobTemplateID] = t.ID
	return t, nil
}

func (r *MemoryWeaveTeacherRepo) GetByMobTemplateID(_ context.Context, mobTemplateID int64) (WeaveTeacher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMobTpl[mobTemplateID]
	if !ok {
		return WeaveTeacher{}, ErrWeaveTeacherNotFound
	}
	return r.teachers[id], nil
}

func (r *MemoryWeaveTeacherRepo) ListWeaveTeachers(_ context.Context) ([]WeaveTeacher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WeaveTeacher, 0, len(r.teachers))
	for _, t := range r.teachers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
