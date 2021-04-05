package docker

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncodeCredentials(t *testing.T) {
	// regression test for https://github.com/anchore/grype/issues/254
	// the JSON encoded credentials should NOT escape characters

	user, pass := "dockerusertest", "WL[cC-<sN#K(zk~NVspmw.PL)3K?v"
	// encoded string: expected := base64encode(`{"password":"WL[cC-<sN#K(zk~NVspmw.PL)3K?v","username":"dockerusertest"}\n`)
	// where the problem character is the "<" within the password, which should NOT be encoded to \u003c
	expected := "eyJwYXNzd29yZCI6IldMW2NDLTxzTiNLKHprfk5Wc3Btdy5QTCkzSz92IiwidXNlcm5hbWUiOiJkb2NrZXJ1c2VydGVzdCJ9Cg=="
	actual, err := encodeCredentials(user, pass)
	if err != nil {
		t.Fatalf("unable to encode credentials: %+v", err)
	}

	assert.Equal(t, expected, actual, "unexpected output")
}
