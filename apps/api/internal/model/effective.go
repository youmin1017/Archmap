package model

import "github.com/google/uuid"

// EffectiveOwner is the owner that applies to an element in one role and the
// element the assignment comes from (the element itself or an ancestor).
type EffectiveOwner struct {
	OwnerID uuid.UUID
	Source  uuid.UUID
}

// Effective holds the values an element inherits from its ancestors (ADR-064).
type Effective struct {
	// Environment is the element's own environment, else the nearest ancestor's ("" = none).
	Environment Environment
	// EnvironmentSource is the element that sets Environment (nil when none does).
	EnvironmentSource *uuid.UUID
	Owners            map[OwnerRole]EffectiveOwner
	// DeployedVia is the element (itself or an ancestor) that has a deployed_on relation; nil when none.
	DeployedVia *uuid.UUID
}

// Owner returns the effective owner in a role.
func (e Effective) Owner(role OwnerRole) (EffectiveOwner, bool) {
	o, ok := e.Owners[role]
	return o, ok
}

// ComputeEffective derives inherited values for every element of s. Ownerships whose
// owner no longer exists are ignored. Parent cycles (rejected elsewhere) end the walk.
func ComputeEffective(s Snapshot) map[uuid.UUID]Effective {
	byID := make(map[uuid.UUID]Element, len(s.Elements))
	for _, e := range s.Elements {
		byID[e.ID] = e
	}
	owners := make(map[uuid.UUID]bool, len(s.Owners))
	for _, o := range s.Owners {
		owners[o.ID] = true
	}
	own := map[uuid.UUID]map[OwnerRole]uuid.UUID{}
	for _, o := range s.Ownerships {
		if !owners[o.OwnerID] {
			continue
		}
		if own[o.ElementID] == nil {
			own[o.ElementID] = map[OwnerRole]uuid.UUID{}
		}
		own[o.ElementID][o.Role] = o.OwnerID
	}
	deployed := map[uuid.UUID]bool{}
	for _, r := range s.Relations {
		if r.Kind == RelDeployedOn {
			deployed[r.SourceID] = true
		}
	}

	out := make(map[uuid.UUID]Effective, len(s.Elements))
	visiting := map[uuid.UUID]bool{}
	var walk func(id uuid.UUID) Effective
	walk = func(id uuid.UUID) Effective {
		if eff, done := out[id]; done {
			return eff
		}
		e := byID[id]
		var inherited Effective
		if e.ParentID != nil && !visiting[id] {
			if _, ok := byID[*e.ParentID]; ok {
				visiting[id] = true
				inherited = walk(*e.ParentID)
				delete(visiting, id)
			}
		}
		eff := Effective{
			Environment:       inherited.Environment,
			EnvironmentSource: inherited.EnvironmentSource,
			Owners:            make(map[OwnerRole]EffectiveOwner, len(OwnerRoles)),
			DeployedVia:       inherited.DeployedVia,
		}
		for role, o := range inherited.Owners {
			eff.Owners[role] = o
		}
		if e.Environment != EnvUnset {
			eff.Environment = e.Environment
			eff.EnvironmentSource = &e.ID
		}
		for role, ownerID := range own[id] {
			eff.Owners[role] = EffectiveOwner{OwnerID: ownerID, Source: id}
		}
		if deployed[id] {
			eff.DeployedVia = &e.ID
		}
		out[id] = eff
		return eff
	}
	for _, e := range s.Elements {
		walk(e.ID)
	}
	return out
}
