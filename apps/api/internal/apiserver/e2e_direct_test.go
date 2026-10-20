package apiserver_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
)

var utcStamp = regexp.MustCompile(`"updated_at":"[^"]+Z"`)

func TestHTTPReviewPolicyAndDirectEdits(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"

	var s api.Settings
	c.expect(c.do("GET", "/api/v1/settings", viewer, nil, &s), http.StatusOK, jsonCT)
	if s.ReviewPolicy != api.ReviewPolicyRequired {
		t.Fatalf("default policy = %q", s.ReviewPolicy)
	}

	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, jsonCT)
	var apiID uuid.UUID
	for _, e := range els.Elements {
		if e.Name == "API" {
			apiID = e.Id
		}
	}
	rename := obj{"summary": "Rename API", "changes": []obj{
		{"op": "update", "target_type": "element", "target_id": apiID, "patch": obj{"name": "Orders API"}},
	}}

	// Required policy: main is read-only; only maintainers change the policy.
	c.expect(c.do("POST", "/api/v1/model/changes", editor, rename, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("PUT", "/api/v1/settings", editor, obj{"review_policy": "direct"}, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("PUT", "/api/v1/settings", maintainer, obj{"review_policy": "sometimes"}, nil), http.StatusBadRequest, problemCT)
	c.expect(c.do("PUT", "/api/v1/settings", maintainer, obj{"review_policy": "direct"}, &s), http.StatusOK, jsonCT)
	if s.ReviewPolicy != api.ReviewPolicyDirect || s.UpdatedBy == nil {
		t.Fatalf("settings = %+v", s)
	}
	// Timestamps are UTC like everywhere else (the web client validates RFC 3339 with Z).
	if body := c.do("GET", "/api/v1/settings", viewer, nil, nil).Body.String(); !utcStamp.MatchString(body) {
		t.Fatalf("updated_at not UTC: %s", body)
	}

	// Direct: an editor's save is a new version; viewers still cannot edit.
	c.expect(c.do("POST", "/api/v1/model/changes", viewer, rename, nil), http.StatusForbidden, problemCT)
	var res api.DirectResult
	c.expect(c.do("POST", "/api/v1/model/changes", editor, rename, &res), http.StatusCreated, jsonCT)
	if res.Outcome != api.DirectOutcomeMerged || res.Version != 2 || res.Changeset.Kind != api.ChangesetKindDirect {
		t.Fatalf("result = %+v", res)
	}
	var vs api.VersionList
	c.expect(c.do("GET", "/api/v1/versions", viewer, nil, &vs), http.StatusOK, jsonCT)
	if vs.Versions[0].Summary != "Rename API" || vs.Versions[0].Kind != api.ChangesetKindDirect {
		t.Fatalf("versions = %+v", vs.Versions[0])
	}
	var list api.ChangesetList
	c.expect(c.do("GET", "/api/v1/changesets", viewer, nil, &list), http.StatusOK, jsonCT)
	for _, cs := range list.Changesets {
		if cs.Kind == api.ChangesetKindDirect {
			t.Fatalf("direct edit listed with drafts: %+v", cs)
		}
	}

	// Undo it: a new version that reverses it, once.
	var undo api.DirectResult
	revert := "/api/v1/model/changes/" + res.Changeset.Id.String() + "/revert"
	c.expect(c.do("POST", revert, editor, nil, &undo), http.StatusCreated, jsonCT)
	if undo.Outcome != api.DirectOutcomeMerged || undo.Version != 3 || undo.Changeset.Reverts == nil || *undo.Changeset.Reverts != res.Changeset.Id {
		t.Fatalf("undo = %+v", undo)
	}
	c.expect(c.do("POST", revert, editor, nil, nil), http.StatusConflict, problemCT)

	// Back to required: direct edits stop.
	c.expect(c.do("PUT", "/api/v1/settings", maintainer, obj{"review_policy": "required"}, nil), http.StatusOK, jsonCT)
	c.expect(c.do("POST", "/api/v1/model/changes", editor, rename, nil), http.StatusForbidden, problemCT)
}
