package apiserver_test

import (
	"net/http"
	"regexp"
	"testing"
)

// Timestamps must be UTC with a Z suffix: the generated Zod validators in the
// TS client reject numeric offsets such as +08:00.
func TestHTTPTimestampsAreUTC(t *testing.T) {
	c, editor := newClient(t)
	var created struct {
		ID string `json:"id"`
	}
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "tz"}, &created), http.StatusCreated, "application/json")

	offset := regexp.MustCompile(`\d{2}:\d{2}:\d{2}(\.\d+)?[+-]\d{2}:\d{2}"`)
	for _, path := range []string{
		"/api/v1/model/elements",
		"/api/v1/model/relations",
		"/api/v1/changesets",
		"/api/v1/changesets/" + created.ID,
		"/api/v1/versions",
	} {
		rec := c.do("GET", path, editor, nil, nil)
		c.expect(rec, http.StatusOK, "application/json")
		if offset.Match(rec.Body.Bytes()) {
			t.Errorf("%s returns a non-UTC timestamp: %s", path, rec.Body)
		}
	}
}
