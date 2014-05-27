package knuckles

import (
  "io"
  "net"
  "net/http"
  "strconv"
  "time"
)

type HTTPProxy struct {
  Server http.Server
}

func NewHTTPProxy() (*HTTPProxy, error) {
  h := &HTTPProxy{}
  mux := http.NewServeMux()
  mux.Handle("/", h)
  h.Server.Handler = mux

  return h, nil
}

func (h *HTTPProxy) Run() error {
  return h.Server.ListenAndServe()
}

func (h *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}

func (h *HTTPProxy) simpleProxy(w http.ResponseWriter, r *http.Request) {

  tr := &http.Transport{
    DisableKeepAlives: true,
  }

  resp, _ := tr.RoundTrip(r)

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
    return
  }

  client, _, err := hj.Hijack()
  if err != nil {
    return
  }
  defer client.Close()

  server, err := net.Dial("tcp", r.URL.Host)
  if err != nil {
    return
  }
  defer server.Close()

  err = r.Write(server)
  if err != nil {
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
