// Command fakedav is an in-memory WebDAV server for the browser end-to-end
// test (B69). It is not part of the release.
//
//	fakedav -addr 127.0.0.1:9000 -user me -password pw
//
// The share is at http://<addr>/dav/ and starts with /docs/hello.txt.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"golang.org/x/net/webdav"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	user := flag.String("user", "me", "user name")
	password := flag.String("password", "pw", "password")
	flag.Parse()

	fs := webdav.NewMemFS()
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/docs", 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := fs.OpenFile(ctx, "/docs/hello.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	_, _ = f.Write([]byte("hello from fakedav"))
	f.Close()

	h := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	log.Fatal(http.ListenAndServe(*addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != *user || p != *password {
			w.Header().Set("WWW-Authenticate", `Basic realm="fakedav"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})))
}
