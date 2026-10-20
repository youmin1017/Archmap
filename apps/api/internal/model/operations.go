package model

import (
	"net/url"
	"strings"
	"time"

	"archmap/api/internal/platform/apperr"
)

// ElementType refines an element's kind (ADR-062). It drives the datastore
// (cylinder) rendering and lets a database server (container) hold logical
// databases (components).
type ElementType string

const (
	TypeNone           ElementType = ""
	TypeDatabaseServer ElementType = "database-server"
	TypeDatabase       ElementType = "database"
	TypeSchema         ElementType = "schema"
	TypeCache          ElementType = "cache"
	TypeQueue          ElementType = "queue"
	TypeProxy          ElementType = "proxy"
)

// ElementTypes lists the valid types in display order.
var ElementTypes = []ElementType{TypeDatabaseServer, TypeDatabase, TypeSchema, TypeCache, TypeQueue, TypeProxy}

func (t ElementType) Valid() bool {
	if t == TypeNone {
		return true
	}
	for _, v := range ElementTypes {
		if t == v {
			return true
		}
	}
	return false
}

// IsDatastore reports whether the type is drawn as a datastore (cylinder).
func (t ElementType) IsDatastore() bool {
	return t == TypeDatabaseServer || t == TypeDatabase || t == TypeSchema
}

// TypeKinds says which kinds a type fits (checked by lint, ADR-062).
var TypeKinds = map[ElementType][]ElementKind{
	TypeDatabaseServer: {KindContainer},
	TypeDatabase:       {KindContainer, KindComponent},
	TypeSchema:         {KindComponent},
	TypeCache:          {KindContainer, KindComponent},
	TypeQueue:          {KindContainer, KindComponent},
	TypeProxy:          {KindContainer, KindComponent, KindNode}, // a reverse proxy is often infrastructure
}

type Environment string

const (
	EnvUnset   Environment = ""
	EnvProd    Environment = "prod"
	EnvStaging Environment = "staging"
	EnvDev     Environment = "dev"
)

func (e Environment) Valid() bool {
	return e == EnvUnset || e == EnvProd || e == EnvStaging || e == EnvDev
}

type Lifecycle string

const (
	LifecycleActive     Lifecycle = "active"
	LifecycleDeprecated Lifecycle = "deprecated"
	LifecyclePlanned    Lifecycle = "planned"
)

func (l Lifecycle) Valid() bool {
	return l == LifecycleActive || l == LifecycleDeprecated || l == LifecyclePlanned
}

type Criticality string

const (
	CriticalityUnset    Criticality = ""
	CriticalityLow      Criticality = "low"
	CriticalityMedium   Criticality = "medium"
	CriticalityHigh     Criticality = "high"
	CriticalityCritical Criticality = "critical"
)

func (c Criticality) Valid() bool {
	switch c {
	case CriticalityUnset, CriticalityLow, CriticalityMedium, CriticalityHigh, CriticalityCritical:
		return true
	}
	return false
}

// HumanOwnedFields are element fields that only people change: a connector sync
// (M12) may create elements and update technical fields but never writes these,
// nor any owner or ownership (ADR-067).
var HumanOwnedFields = []string{"environment", "lifecycle"}

const (
	maxURLLength = 2048
	dateLayout   = time.DateOnly
)

// normalized applies the defaults and legacy mappings every element read passes through:
// the retired kind "database" becomes a container of type "database", and an empty
// lifecycle (elements stored before M10) means active.
func (e Element) normalized() Element {
	if e.Kind == KindDatabase {
		e.Kind = KindContainer
		if e.Type == TypeNone {
			e.Type = TypeDatabase
		}
	}
	if e.Lifecycle == "" {
		e.Lifecycle = LifecycleActive
	}
	if e.Projects == nil {
		e.Projects = []string{}
	}
	e.RunbookURL = strings.TrimSpace(e.RunbookURL)
	e.DocsURL = strings.TrimSpace(e.DocsURL)
	return e
}

// Normalized returns e with legacy values mapped (for rows and snapshots read from storage).
func (e Element) Normalized() Element { return e.normalized() }

func (e Element) validateOperations() error {
	if !e.Type.Valid() {
		return apperr.Invalid("invalid element type %q", e.Type)
	}
	if !e.Environment.Valid() {
		return apperr.Invalid("environment must be prod, staging, dev or empty")
	}
	if !e.Lifecycle.Valid() {
		return apperr.Invalid("lifecycle must be active, deprecated or planned")
	}
	if !e.Criticality.Valid() {
		return apperr.Invalid("criticality must be low, medium, high, critical or empty")
	}
	for name, u := range map[string]string{"runbook_url": e.RunbookURL, "docs_url": e.DocsURL} {
		if err := ValidateURL(name, u); err != nil {
			return err
		}
	}
	if e.LastReviewedAt != nil {
		if _, err := time.Parse(dateLayout, *e.LastReviewedAt); err != nil {
			return apperr.Invalid("last_reviewed_at must be a date (YYYY-MM-DD)")
		}
	}
	return nil
}

// ValidateURL accepts "" or an absolute http(s) URL.
func ValidateURL(field, u string) error {
	if u == "" {
		return nil
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		len(u) > maxURLLength || strings.ContainsAny(u, " \r\n\t") {
		return apperr.Invalid("%s must be an http(s) URL", field)
	}
	return nil
}

// ReviewedAt parses LastReviewedAt (zero time when never reviewed).
func (e Element) ReviewedAt() (time.Time, bool) {
	if e.LastReviewedAt == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(dateLayout, *e.LastReviewedAt)
	return t, err == nil
}
