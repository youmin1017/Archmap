package apiserver_test

import (
	"fmt"
	"net/http"
	"testing"

	"archmap/api/internal/gen/api"
)

func TestHTTPUserManagement(t *testing.T) {
	c, editor := newClient(t)
	const maintainer, viewer = "maintainer@test.local", "viewer@test.local"
	j, p := "application/json", "application/problem+json"

	// Only maintainers see or change users.
	c.expect(c.do("GET", "/api/v1/users", editor, nil, nil), http.StatusForbidden, p)
	c.expect(c.do("GET", "/api/v1/users", "", nil, nil), http.StatusUnauthorized, p)
	var list api.UserList
	c.expect(c.do("GET", "/api/v1/users", maintainer, nil, &list), http.StatusOK, j)
	byEmail := map[string]api.User{}
	for _, u := range list.Users {
		byEmail[u.Email] = u
	}
	if len(byEmail) < 3 {
		t.Fatalf("users = %+v", list.Users)
	}

	role := func(id any) string { return "/api/v1/users/" + fmt.Sprint(id) + "/role" }
	v := byEmail[viewer]
	c.expect(c.do("PUT", role(v.Id), editor, obj{"role": "maintainer"}, nil), http.StatusForbidden, p)
	c.expect(c.do("PUT", role(v.Id), maintainer, obj{"role": "root"}, nil), http.StatusBadRequest, p)
	c.expect(c.do("PUT", role("00000000-0000-0000-0000-000000000000"), maintainer, obj{"role": "editor"}, nil), http.StatusNotFound, p)

	var got api.User
	c.expect(c.do("PUT", role(v.Id), maintainer, obj{"role": "editor"}, &got), http.StatusOK, j)
	if got.Role != "editor" {
		t.Fatalf("role = %s", got.Role)
	}

	// The last maintainer cannot be demoted; with a second one it can.
	m := byEmail[maintainer]
	c.expect(c.do("PUT", role(m.Id), maintainer, obj{"role": "editor"}, nil), http.StatusConflict, p)
	c.expect(c.do("PUT", role(v.Id), maintainer, obj{"role": "maintainer"}, nil), http.StatusOK, j)
	c.expect(c.do("PUT", role(m.Id), maintainer, obj{"role": "editor"}, nil), http.StatusOK, j)
}
