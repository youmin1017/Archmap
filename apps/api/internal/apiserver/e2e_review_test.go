package apiserver_test

import (
	"net/http"
	"testing"

	"archmap/api/internal/gen/api"
)

func TestHTTPReviewAndMerge(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"
	j, p := "application/json", "application/problem+json"

	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements?q=api", viewer, nil, &els), http.StatusOK, j)
	apiID := els.Elements[0].Id

	newDraft := func(title string, patch obj) string {
		var cs api.Changeset
		c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": title}, &cs), http.StatusCreated, j)
		c.expect(c.do("POST", "/api/v1/changesets/"+cs.Id.String()+"/changes", editor, obj{"changes": []obj{
			{"op": "update", "target_type": "element", "target_id": apiID, "patch": patch},
		}}, nil), http.StatusCreated, j)
		return "/api/v1/changesets/" + cs.Id.String()
	}
	a := newDraft("A", obj{"name": "Theirs"})
	b := newDraft("B", obj{"name": "Mine"})

	// Diff and lint on a draft.
	var diff api.Diff
	c.expect(c.do("GET", a+"/diff", viewer, nil, &diff), http.StatusOK, j)
	if len(diff.Elements.Modified) != 1 || diff.Elements.Modified[0].Fields[0].Field != "name" || diff.Stale {
		t.Fatalf("diff = %+v", diff)
	}
	var report api.LintReport
	c.expect(c.do("GET", a+"/lint", viewer, nil, &report), http.StatusOK, j)
	if report.Errors != 0 {
		t.Fatalf("lint = %+v", report)
	}

	// Review workflow guards.
	c.expect(c.do("POST", a+"/merge", maintainer, nil, nil), http.StatusConflict, p) // still a draft
	c.expect(c.do("POST", a+"/submit", viewer, nil, nil), http.StatusForbidden, p)
	var cs api.Changeset
	c.expect(c.do("POST", a+"/submit", editor, nil, &cs), http.StatusOK, j)
	if cs.Status != "in_review" {
		t.Fatalf("status = %s", cs.Status)
	}
	c.expect(c.do("POST", a+"/approve", editor, nil, nil), http.StatusForbidden, p)
	c.expect(c.do("POST", a+"/approve", maintainer, nil, &cs), http.StatusOK, j)
	if cs.ApprovedBy == nil {
		t.Fatal("approval not reported")
	}
	var merged api.MergeResult
	c.expect(c.do("POST", a+"/merge", maintainer, nil, &merged), http.StatusOK, j)
	if merged.Version != 2 || merged.Changeset.Status != "merged" {
		t.Fatalf("merge = %+v", merged)
	}

	// The version browser's diff: version 1 -> 2 shows what the merge changed, 2 -> 1 the reverse.
	var vd api.Diff
	c.expect(c.do("GET", "/api/v1/versions/1/diff/2", viewer, nil, &vd), http.StatusOK, j)
	if vd.BaseVersion != 1 || vd.MainVersion != 2 || vd.Stale {
		t.Fatalf("version diff header = %+v", vd)
	}
	if len(vd.Elements.Added)+len(vd.Elements.Removed)+len(vd.Elements.Modified)+len(vd.Relations.Added)+len(vd.Relations.Removed)+len(vd.Relations.Modified) == 0 {
		t.Fatal("version diff is empty although version 2 changed the model")
	}
	c.expect(c.do("GET", "/api/v1/versions/1/diff/1", viewer, nil, &vd), http.StatusOK, j)
	if len(vd.Elements.Modified) != 0 {
		t.Fatalf("same-version diff = %+v", vd)
	}
	c.expect(c.do("GET", "/api/v1/versions/1/diff/99", viewer, nil, nil), http.StatusNotFound, p)

	// B conflicts with A: 409 with the conflict list, main untouched.
	c.expect(c.do("POST", b+"/submit", editor, nil, nil), http.StatusOK, j)
	c.expect(c.do("POST", b+"/approve", maintainer, nil, nil), http.StatusOK, j)
	var conflicts api.ConflictList
	rec := c.do("POST", b+"/merge", maintainer, nil, nil)
	c.expect(rec, http.StatusConflict, j)
	c.decode(rec, &conflicts)
	if len(conflicts.Conflicts) != 1 || conflicts.Conflicts[0].Field != "name" ||
		conflicts.Conflicts[0].Mine != "Mine" || conflicts.Conflicts[0].Theirs != "Theirs" || conflicts.Conflicts[0].Base != "API" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	var cl api.ConflictList
	c.expect(c.do("GET", b+"/conflicts", viewer, nil, &cl), http.StatusOK, j)
	if len(cl.Conflicts) != 1 {
		t.Fatalf("conflicts endpoint = %+v", cl)
	}

	// Resolve, then merge.
	c.expect(c.do("POST", b+"/resolve", editor, obj{"resolutions": []obj{}}, nil), http.StatusBadRequest, p)
	c.expect(c.do("POST", b+"/resolve", editor, obj{"resolutions": []obj{
		{"target_type": "element", "target_id": apiID, "field": "name", "choice": "ours"},
	}}, nil), http.StatusBadRequest, p) // invalid enum rejected by the spec validator
	c.expect(c.do("POST", b+"/resolve", editor, obj{"resolutions": []obj{
		{"target_type": "element", "target_id": apiID, "field": "name", "choice": "mine"},
	}}, &cl), http.StatusOK, j)
	if len(cl.Conflicts) != 0 {
		t.Fatalf("remaining = %+v", cl)
	}
	c.expect(c.do("POST", b+"/merge", maintainer, nil, &merged), http.StatusOK, j)
	if merged.Version != 3 {
		t.Fatalf("merge b = %+v", merged)
	}

	// History.
	var versions api.VersionList
	c.expect(c.do("GET", "/api/v1/versions", viewer, nil, &versions), http.StatusOK, j)
	if len(versions.Versions) != 3 || versions.Versions[0].Number != 3 {
		t.Fatalf("versions = %+v", versions.Versions)
	}
	var v1, v3 api.Model
	c.expect(c.do("GET", "/api/v1/versions/1/model", viewer, nil, &v1), http.StatusOK, j)
	c.expect(c.do("GET", "/api/v1/versions/3/model", viewer, nil, &v3), http.StatusOK, j)
	nameOf := func(m api.Model) string {
		for _, e := range m.Elements {
			if e.Id == apiID {
				return e.Name
			}
		}
		return ""
	}
	if nameOf(v1) != "API" || nameOf(v3) != "Mine" {
		t.Fatalf("names v1=%q v3=%q", nameOf(v1), nameOf(v3))
	}
	c.expect(c.do("GET", "/api/v1/versions/99/model", viewer, nil, nil), http.StatusNotFound, p)
	c.expect(c.do("GET", "/api/v1/versions/0/model", viewer, nil, nil), http.StatusBadRequest, p) // minimum: 1 in the spec

	// Reject and abandon.
	r := newDraft("R", obj{"description": "x"})
	c.expect(c.do("POST", r+"/submit", editor, nil, nil), http.StatusOK, j)
	c.expect(c.do("POST", r+"/reject", editor, nil, nil), http.StatusForbidden, p)
	c.expect(c.do("POST", r+"/reject", maintainer, nil, &cs), http.StatusOK, j)
	if cs.Status != "rejected" {
		t.Fatalf("status = %s", cs.Status)
	}
	ab := newDraft("Ab", obj{"description": "y"})
	c.expect(c.do("POST", ab+"/abandon", editor, nil, &cs), http.StatusOK, j)
	if cs.Status != "abandoned" {
		t.Fatalf("status = %s", cs.Status)
	}
}
