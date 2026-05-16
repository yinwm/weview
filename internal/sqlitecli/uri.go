package sqlitecli

import (
	"net/url"
	"path/filepath"
	"strings"
)

func ImmutableURI(path string) string {
	path = filepath.ToSlash(path)
	if vol := filepath.VolumeName(path); vol != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "mode=ro&immutable=1",
	}
	return u.String()
}
