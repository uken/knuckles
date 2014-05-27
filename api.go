package knuckles

import (
  "net"
  "net/http"
  "strconv"
)

type HTTPAPIConfig struct {
  Store Store
  Addr  string
}

type HTTPAPI struct {
  Server   http.Server
  Db       Store
  listener net.Listener
  Addr     string
}

func NewHTTPAPI(config HTTPAPIConfig) (*HTTPAPI, error) {
  h := &HTTPAPI{
    Db:   config.Store,
    Addr: config.Addr,
  }

  mux := http.NewServeMux()
  mux.HandleFunc("/status", h.ServeStatus)
  mux.HandleFunc("/api", h.ServeAPI)
  h.Server.Handler = mux

  return h, nil
}

func (h *HTTPAPI) Start() error {
  var err error

  h.listener, err = net.Listen("tcp", h.Addr)
  if err != nil {
    return err
  }

  return h.Server.Serve(h.listener)
}

func (h *HTTPAPI) Stop() error {
  return h.listener.Close()
}

func (h *HTTPAPI) ServeStatus(w http.ResponseWriter, r *http.Request) {
}

func (h *HTTPAPI) ServeAPI(w http.ResponseWriter, r *http.Request) {
  var err error
  action := r.FormValue("action")
  app := r.FormValue("application")
  backend := r.FormValue("backend")
  hostname := r.FormValue("hostname")
  ttlRaw := r.FormValue("ttl")
  ttl, _ := strconv.Atoi(ttlRaw)

  switch action {
  case "add-application":
    err = h.Db.AddApplication(app)
  case "add-hostname":
    err = h.Db.AddHostname(app, hostname)
  case "add-backend":
    err = h.Db.AddBackend(app, backend, ttl)

  case "del-application":
    err = h.Db.RemoveApplication(app)
  case "del-hostname":
    err = h.Db.RemoveHostname(app, hostname)
  case "del-backend":
    err = h.Db.RemoveBackend(app, backend)

  case "list-application":
  case "list-backend":
  case "list-hostname":

  default:
    err = ErrInvalidAction
  }

  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
  }
}
