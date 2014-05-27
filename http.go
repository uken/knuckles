package knuckles

import (
  "fmt"
  "io"
  "log"
  "net"
  "net/http"
  "net/url"
  "strconv"
  "strings"
  "time"
)

type HTTPProxyConfig struct {
  XForwardedFor         bool
  XRequestStart         bool
  XForwardedProto       string
  Store                 Store
  Addr                  string
  RedirectNoHostname    string
  RedirectNoBackend     string
  RedirectInternalError string
}

type HTTPProxy struct {
  Server   http.Server
  listener net.Listener
  Config   HTTPProxyConfig
}

func NewHTTPProxy(config HTTPProxyConfig) (*HTTPProxy, error) {
  h := &HTTPProxy{
    Config: config,
  }

  mux := http.NewServeMux()
  mux.Handle("/", h)
  h.Server.Handler = mux

  return h, nil
}

func (h *HTTPProxy) Start() error {
  var err error

  h.listener, err = net.Listen("tcp", h.Config.Addr)
  if err != nil {
    return err
  }

  return h.Server.Serve(h.listener)
}

func (h *HTTPProxy) Stop() error {
  return h.listener.Close()
}

func (h *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  hostname := r.Host
  log.Println("Request for", hostname)

  if sep := strings.Index(hostname, ":"); sep >= 0 {
    hostname = hostname[:sep]
  }

  // tag before starting redis queries
  if h.Config.XRequestStart {
    r.Header.Set("X-Request-Start", requestStart())
  }

  if h.Config.XForwardedProto != "" {
    r.Header.Set("X-Forwarded-Proto", h.Config.XForwardedProto)
  }

  if h.Config.XForwardedFor {
    r.Header.Set("X-Forwarded-For", clientIP(r))
  }

  endpoint, err := h.Config.Store.EndpointForHostname(hostname)

  if err != nil {
    h.clientErr(w, r, err)
    return
  }

  r.URL.Host = endpoint
  r.URL.Scheme = "http"

  connection := r.Header.Get("Connection")

  if strings.ToLower(connection) == "upgrade" {
    h.wsProxy(w, r)
  } else {
    h.simpleProxy(w, r)
  }
}

func (h *HTTPProxy) simpleProxy(w http.ResponseWriter, r *http.Request) {

  tr := &http.Transport{
    DisableKeepAlives: true,
  }

  resp, err := tr.RoundTrip(r)

  if err != nil {
    h.clientErr(w, r, err)
    return
  }

  defer resp.Body.Close()

  for name, values := range resp.Header {
    for _, val := range values {
      w.Header().Add(name, val)
    }
  }

  w.WriteHeader(resp.StatusCode)
  // TODO: check copy
  io.Copy(w, resp.Body)
}

func (h *HTTPProxy) wsProxy(w http.ResponseWriter, r *http.Request) {
  hj, ok := w.(http.Hijacker)

  if !ok {
    h.clientErr(w, r, ErrInvalidAction)
    return
  }

  client, _, err := hj.Hijack()
  if err != nil {
    h.clientErr(w, r, err)
    return
  }
  defer client.Close()

  server, err := net.Dial("tcp", r.URL.Host)
  if err != nil {
    h.clientErr(w, r, err)
    return
  }
  defer server.Close()

  err = r.Write(server)
  if err != nil {
    h.clientErr(w, r, err)
    return
  }

  go passBytes(client, server)
}

func passBytes(client, server net.Conn) {
  pass := func(from, to net.Conn, done chan error) {
    _, err := io.Copy(to, from)
    done <- err
  }

  done := make(chan error, 2)

  go pass(client, server, done)
  go pass(server, client, done)

  <-done
  client.Close()
  server.Close()
  <-done
  close(done)
}

func requestStart() string {
  return strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
}

func clientIP(r *http.Request) string {
  raddr := strings.Split(r.RemoteAddr, ":")

  if len(raddr) == 0 {
    return ""
  }

  return raddr[0]
}

func (h *HTTPProxy) clientErr(w http.ResponseWriter, r *http.Request, inputErr error) {
  var redirect string

  switch inputErr {
  case ErrNoBackend:
    redirect = h.Config.RedirectNoBackend
  case ErrDeadBackend:
    redirect = h.Config.RedirectNoBackend
  case ErrNoHostname:
    redirect = h.Config.RedirectNoHostname
  default:
    redirect = h.Config.RedirectInternalError
  }

  finalURL := fmt.Sprintf("%s?err=%s", redirect, url.QueryEscape(inputErr.Error()))
  http.Redirect(w, r, finalURL, http.StatusTemporaryRedirect)
}
