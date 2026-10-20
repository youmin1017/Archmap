package password

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := Hash("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"correct", "s3cret", true},
		{"wrong", "other", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Verify(tc.password, hash)
			if err != nil || got != tc.want {
				t.Fatalf("Verify = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	if _, err := Verify("x", "not-a-hash"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}
