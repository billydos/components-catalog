package storage

import "testing"

func TestMaskDSN(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{"empty", "", ""},
		{"url with password", "postgres://user:secret@host:5432/db",
			"postgres://user:***@host:5432/db"},
		{"url with params", "postgres://user:p@ss@host/db?sslmode=disable",
			"postgres://user:***@host/db?sslmode=disable"},
		{"url without password", "postgres://user@host/db",
			"postgres://user@host/db"},
		{"url with empty password", "postgres://user:@host/db",
			"postgres://user:@host/db"},
		{"sqlite file", "/path/to/catalog.db", "/path/to/catalog.db"},
		{"keyword password", "host=localhost user=u password=secret dbname=db",
			"host=localhost user=u password=*** dbname=db"},
		{"keyword quoted", "host=h password='two words' dbname=d",
			"host=h password=*** dbname=d"},
		{"keyword no password", "host=h user=u dbname=d",
			"host=h user=u dbname=d"},
		{"keyword inner match", "host=h xpassword=keep dbname=d",
			"host=h xpassword=keep dbname=d"},
		{"keyword at start", "password=secret host=h", "password=*** host=h"},
		{"keyword escaped space", `host=h user=u password=a\ b dbname=d`,
			`host=h user=u password=*** dbname=d`},
		{"keyword repeated", "host=h password=FIRST password=SECOND dbname=d",
			"host=h password=*** password=*** dbname=d"},
		{"url userinfo without host", "postgres://user:secret", "***"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaskDSN(tc.dsn); got != tc.want {
				t.Fatalf("MaskDSN(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
		})
	}
}
