// Package webui встраивает веб-интерфейс в бинарник: сервер работает без интернета и CDN.
package webui

import "embed"

//go:embed *.html *.js *.css *.svg
var Files embed.FS
