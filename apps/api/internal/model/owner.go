package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
)

// Owners and ownerships are model data (ADR-063): they change only through
// drafts and are diffed, merged and versioned like elements. Owners are not
// login users; a later mapping to identity-provider groups is a separate concern.

type OwnerKind string

const (
	// OwnerTeam and OwnerPerson are named owners without an account.
	OwnerTeam   OwnerKind = "team"
	OwnerPerson OwnerKind = "person"
	// OwnerUser and OwnerGroup point at a login user or an app-managed group (ADR-069).
	OwnerUser  OwnerKind = "user"
	OwnerGroup OwnerKind = "group"
)

func (k OwnerKind) Valid() bool {
	return k == OwnerTeam || k == OwnerPerson || k == OwnerUser || k == OwnerGroup
}

// IsPrincipal reports whether owners of this kind point at a user or group.
func (k OwnerKind) IsPrincipal() bool { return k == OwnerUser || k == OwnerGroup }

type Owner struct {
	ID        uuid.UUID `json:"id"`
	Kind      OwnerKind `json:"kind"`
	Name      string    `json:"name"`
	Contact   string    `json:"contact"`
	OncallURL string    `json:"oncall_url"`
	// PrincipalID is the user or group of a user / group owner; nil for named owners.
	PrincipalID *uuid.UUID `json:"principal_id"`
	Version     int        `json:"version"`
	DeletedAt   *time.Time `json:"-"`
	CreatedAt   time.Time  `json:"-"`
	UpdatedAt   time.Time  `json:"-"`
}

type OwnerRole string

const (
	RoleOwner      OwnerRole = "owner"
	RoleMaintainer OwnerRole = "maintainer"
	RoleOncall     OwnerRole = "oncall"
	RoleEscalation OwnerRole = "escalation"
)

// OwnerRoles lists the roles in display order.
var OwnerRoles = []OwnerRole{RoleOwner, RoleMaintainer, RoleOncall, RoleEscalation}

func (r OwnerRole) Valid() bool {
	for _, v := range OwnerRoles {
		if r == v {
			return true
		}
	}
	return false
}

// Ownership assigns an owner to an element in one role; at most one per (element, role).
type Ownership struct {
	ID        uuid.UUID  `json:"id"`
	ElementID uuid.UUID  `json:"element_id"`
	OwnerID   uuid.UUID  `json:"owner_id"`
	Role      OwnerRole  `json:"role"`
	Version   int        `json:"version"`
	DeletedAt *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
}

var (
	OwnerPatchFields     = []string{"kind", "name", "contact", "oncall_url", "principal_id"}
	OwnershipPatchFields = []string{"element_id", "owner_id", "role"}
)

// ownershipNamespace seeds the deterministic ownership ids.
var ownershipNamespace = uuid.MustParse("6f1d2a8e-3c4b-5d7e-9f00-a1b2c3d4e5f6")

// OwnershipID is the id of the (element, role) assignment. Being deterministic, two
// drafts that assign the same role on the same element touch the same target, so a
// 3-way merge reports a conflict on owner_id instead of keeping one silently (ADR-063).
func OwnershipID(elementID uuid.UUID, role OwnerRole) uuid.UUID {
	return uuid.NewSHA1(ownershipNamespace, append(elementID[:], []byte(role)...))
}

// principalOwnerNamespace seeds the deterministic ids of user / group owners.
var principalOwnerNamespace = uuid.MustParse("0d6b8f3a-9e2c-5b41-8a7d-3f5e6c1b2a90")

// PrincipalOwnerID is the id of the owner that stands for a user or group: one per
// principal, the same in every draft, so concurrent drafts that introduce it merge
// cleanly (ADR-069).
func PrincipalOwnerID(principal uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(principalOwnerNamespace, principal[:])
}

const (
	maxOwnerName    = 120
	maxOwnerContact = 200
)

func (o Owner) Validate() error {
	if !o.Kind.Valid() {
		return apperr.Invalid("owner kind must be team or person")
	}
	name := strings.TrimSpace(o.Name)
	if name == "" || len([]rune(name)) > maxOwnerName || strings.ContainsAny(o.Name, "\r\n") {
		return apperr.Invalid("owner name must be 1-%d characters on a single line", maxOwnerName)
	}
	if len([]rune(o.Contact)) > maxOwnerContact || strings.ContainsAny(o.Contact, "\r\n") {
		return apperr.Invalid("owner contact must be a single line of at most %d characters", maxOwnerContact)
	}
	switch {
	case o.Kind.IsPrincipal() && o.PrincipalID == nil:
		return apperr.Invalid("a %s owner needs principal_id", o.Kind)
	case !o.Kind.IsPrincipal() && o.PrincipalID != nil:
		return apperr.Invalid("only user and group owners have a principal_id")
	case o.Kind.IsPrincipal() && o.ID != PrincipalOwnerID(*o.PrincipalID):
		return apperr.Invalid("a %s owner's id must be derived from principal_id", o.Kind)
	}
	return ValidateURL("oncall_url", o.OncallURL)
}

func (o Ownership) Validate() error {
	if !o.Role.Valid() {
		return apperr.Invalid("ownership role must be owner, maintainer, oncall or escalation")
	}
	if o.ElementID == uuid.Nil || o.OwnerID == uuid.Nil {
		return apperr.Invalid("ownership needs element_id and owner_id")
	}
	if o.ID != OwnershipID(o.ElementID, o.Role) {
		return apperr.Invalid("ownership id must be derived from element_id and role")
	}
	return nil
}

func ApplyOwnerPatch(o Owner, patch json.RawMessage) (Owner, error) {
	if err := checkKeys(patch, OwnerPatchFields); err != nil {
		return Owner{}, err
	}
	if o.PrincipalID != nil {
		p := *o.PrincipalID // json.Unmarshal writes through pointers; detach
		o.PrincipalID = &p
	}
	if err := json.Unmarshal(patch, &o); err != nil {
		return Owner{}, apperr.Invalid("invalid owner patch: %v", err)
	}
	o.Name = strings.TrimSpace(o.Name)
	o.Contact = strings.TrimSpace(o.Contact)
	o.OncallURL = strings.TrimSpace(o.OncallURL)
	return o, nil
}

func ApplyOwnershipPatch(o Ownership, patch json.RawMessage) (Ownership, error) {
	if err := checkKeys(patch, OwnershipPatchFields); err != nil {
		return Ownership{}, err
	}
	if err := json.Unmarshal(patch, &o); err != nil {
		return Ownership{}, apperr.Invalid("invalid ownership patch: %v", err)
	}
	return o, nil
}

func (o Owner) FieldMap() map[string]json.RawMessage     { return fieldMap(o, OwnerPatchFields) }
func (o Ownership) FieldMap() map[string]json.RawMessage { return fieldMap(o, OwnershipPatchFields) }

// Principal is a user or group an owner can point at, as the directory knows it now.
type Principal struct {
	ID   uuid.UUID
	Kind OwnerKind // OwnerUser or OwnerGroup
	Name string
	// Inactive: a disabled user or a deleted group.
	Inactive bool
}

// Principals indexes the directory by id.
type Principals map[uuid.UUID]Principal
