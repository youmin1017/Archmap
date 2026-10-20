package auth_test

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/auth"
	"archmap/api/internal/platform/authctx"
)

type memUser struct {
	id       uuid.UUID
	email    string
	name     string
	role     authctx.Role
	disabled bool
}

type memIdentity struct {
	rec      auth.IdentityRecord
	provider string
	issuer   string
	subject  string
	email    string
	last     *time.Time
}

type memSession struct {
	rec      auth.SessionRecord
	lastSeen time.Time
}

// memRepo is an in-memory auth.Repository for service tests.
type memRepo struct {
	mu         sync.Mutex
	users      map[uuid.UUID]*memUser
	identities []*memIdentity
	sessions   map[string]*memSession
	flows      map[string]auth.Flow
}

func newMemRepo() *memRepo {
	return &memRepo{users: map[uuid.UUID]*memUser{}, sessions: map[string]*memSession{}, flows: map[string]auth.Flow{}}
}

// addUser creates a user with one identity and returns the user id.
func (r *memRepo) addUser(name, email string, role authctx.Role, issuer, subject, hash string) uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := &memUser{id: uuid.New(), email: email, name: name, role: role}
	r.users[u.id] = u
	r.identities = append(r.identities, &memIdentity{
		rec:      auth.IdentityRecord{ID: uuid.New(), UserID: u.id, PasswordHash: hash},
		provider: "seed", issuer: issuer, subject: subject, email: email,
	})
	return u.id
}

func (r *memRepo) identitiesOf(userID uuid.UUID) []*memIdentity {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*memIdentity
	for _, i := range r.identities {
		if i.rec.UserID == userID {
			out = append(out, i)
		}
	}
	return out
}

func (r *memRepo) sessionCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

func (r *memRepo) FindIdentity(_ context.Context, issuer, subject string) (auth.IdentityRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.identities {
		if i.issuer == issuer && i.subject == subject {
			u := r.users[i.rec.UserID]
			rec := i.rec
			rec.Role, rec.Disabled = u.role, u.disabled
			return rec, true, nil
		}
	}
	return auth.IdentityRecord{}, false, nil
}

func (r *memRepo) Provision(_ context.Context, id auth.Identity, role authctx.Role) (auth.IdentityRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := &memUser{id: uuid.New(), email: id.Email, name: id.Name, role: role}
	r.users[u.id] = u
	i := &memIdentity{
		rec:      auth.IdentityRecord{ID: uuid.New(), UserID: u.id, Role: role},
		provider: id.Provider, issuer: id.Issuer, subject: id.Subject, email: id.Email,
	}
	r.identities = append(r.identities, i)
	return i.rec, nil
}

func (r *memRepo) RecordLogin(_ context.Context, rec auth.IdentityRecord, _ string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.identities {
		if i.rec.ID == rec.ID {
			i.last = &at
		}
	}
	return nil
}

func (r *memRepo) SetRole(_ context.Context, userID uuid.UUID, role authctx.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[userID].role = role
	return nil
}

func (r *memRepo) CreateSession(_ context.Context, s auth.SessionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[string(s.TokenHash)] = &memSession{rec: s, lastSeen: s.CreatedAt}
	return nil
}

func (r *memRepo) GetSession(_ context.Context, h []byte, now time.Time) (auth.ActiveSession, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[string(h)]
	if !ok || !now.Before(s.rec.IdleExpiresAt) || !now.Before(s.rec.ExpiresAt) {
		return auth.ActiveSession{}, false, nil
	}
	u := r.users[s.rec.UserID]
	if u.disabled {
		return auth.ActiveSession{}, false, nil
	}
	return auth.ActiveSession{
		Actor:     authctx.Actor{ID: u.id, Email: u.email, Name: u.name, Role: u.role, Provider: s.rec.Provider},
		CSRFToken: s.rec.CSRFToken, LastSeenAt: s.lastSeen, ExpiresAt: s.rec.ExpiresAt,
	}, true, nil
}

func (r *memRepo) TouchSession(_ context.Context, h []byte, now, idle time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[string(h)]; ok {
		s.lastSeen = now
		s.rec.IdleExpiresAt = idle
		if idle.After(s.rec.ExpiresAt) {
			s.rec.IdleExpiresAt = s.rec.ExpiresAt
		}
	}
	return nil
}

func (r *memRepo) DeleteSession(_ context.Context, h []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, string(h))
	return nil
}

func (r *memRepo) DeleteOtherSessions(_ context.Context, userID uuid.UUID, keep []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.sessions {
		if s.rec.UserID == userID && k != string(keep) {
			delete(r.sessions, k)
		}
	}
	return nil
}

func (r *memRepo) DeleteExpired(_ context.Context, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.sessions {
		if !now.Before(s.rec.IdleExpiresAt) || !now.Before(s.rec.ExpiresAt) {
			delete(r.sessions, k)
		}
	}
	for k, f := range r.flows {
		if !now.Before(f.ExpiresAt) {
			delete(r.flows, k)
		}
	}
	return nil
}

func (r *memRepo) SaveFlow(_ context.Context, f auth.Flow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flows[f.ID] = f
	return nil
}

func (r *memRepo) TakeFlow(_ context.Context, id string) (auth.Flow, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.flows[id]
	delete(r.flows, id)
	return f, ok, nil
}

func (r *memRepo) LocalPassword(_ context.Context, userID uuid.UUID) (uuid.UUID, string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.identities {
		if i.rec.UserID == userID && i.issuer == auth.LocalIssuer {
			return i.rec.ID, i.rec.PasswordHash, true, nil
		}
	}
	return uuid.Nil, "", false, nil
}

func (r *memRepo) SetPassword(_ context.Context, identityID uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.identities {
		if i.rec.ID == identityID {
			i.rec.PasswordHash = hash
		}
	}
	return nil
}
