package fwserver

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"log"

	"github.com/rs/cors"
)

type FirmwareServer struct {
	server *http.Server
	Base   string
}

type Config struct {
	Port int
	Base string
}

func New(config *Config) (*FirmwareServer, error) {

	c := cors.New(cors.Options{
		AllowOriginFunc:  func(origin string) bool { return true },
		AllowCredentials: true,
	})

	fws := &FirmwareServer{
		Base: config.Base,
	}
	handler := c.Handler(fws)

	fws.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: handler,
	}

	if err := fws.server.ListenAndServe(); err != nil {
		return nil, err
	}

	return fws, nil

}

func (fws *FirmwareServer) Log(r *http.Request, code int, err error, other interface{}) {
	id := r.Header.Get("X-Chip-Id")
	if id == "" {
		id = "?"
	}
	agent := r.Header.Get("User-Agent")
	if agent == "" {
		agent = "?"
	}
	if other == nil {
		other = ""
	}
	log.Printf("%s\t%s\t%s\t%d\t%s\t%s\t%v\n", r.RemoteAddr, id, agent, code, r.URL.Path, err, other)
}

func (fws *FirmwareServer) Serve(w http.ResponseWriter, r *http.Request) error {
	path := filepath.Join(fws.Base, strings.Replace(r.URL.Path, "..", "", -1))
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	hashPath := path + ".hash"
	hash, err := ioutil.ReadFile(hashPath)
	if err != nil {
		return err
	}
	etag := fmt.Sprintf("%q", string(hash))

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		fws.Log(r, 304, nil, nil)
		return nil
	}

	reader, err := os.Open(path)
	if err != nil {
		return err
	}
	w.Header().Add("Etag", etag)
	w.Header().Add("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Add("Content-Type", "application/octet-stream")
	w.Header().Add("X-ETag-Verify", "true")
	_, err = io.Copy(w, reader)
	if err == nil {
		fws.Log(r, 200, nil, fi.Size())
	}
	return err
}

func (fws *FirmwareServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := fws.Serve(w, r)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Error: %s\n", err)))
		fws.Log(r, 503, err, nil)
	}
}
