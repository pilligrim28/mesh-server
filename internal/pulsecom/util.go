package pulsecom

import (
	"errors"
	"net/url"
)

var errBadAction = errors.New("action: start | stop")

func urlPathEscape(s string) string { return url.PathEscape(s) }
