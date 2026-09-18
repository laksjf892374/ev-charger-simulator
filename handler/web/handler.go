// Package web serves the simulator's single-page UI. The page is static: everything it shows and
// does goes through the public control API and the mock eMSP's API.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var embedded embed.FS

func NewHandler() (http.Handler, error) {
	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, err
	}

	return http.FileServerFS(static), nil
}
