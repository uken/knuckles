package knuckles

import (
  "fmt"
  "log"
  "net"
  "net/http"
  "time"
)

type PingWork struct {
  App      string
  Backends []string
}

type Pinger struct {
  q           chan bool
  Store       Store
  interval    int
  workingApps []string
}

func NewPinger(namespace, redis string, interval int) (*Pinger, error) {
  var err error
  p := &Pinger{
    q:        make(chan bool, 1),
    interval: interval,
  }

  p.Store, err = NewRedisStore(namespace, redis)

  return p, err
}

func (pinger *Pinger) Start() error {
  ch := make(chan PingWork)
  go pinger.Feed(ch)

  for {
    w, ok := <-ch
    if !ok {
      break
    }

    pinger.Process(w)
  }

  return nil
}

func (pinger *Pinger) Feed(ch chan PingWork) {
  tick := time.NewTicker(time.Duration(pinger.interval) * time.Second)
  for {
    select {
    case <-pinger.q:
      close(ch)
      break
    case <-tick.C:
      pinger.feedOne(ch)
    }
  }
  tick.Stop()
}

func (pinger *Pinger) feedOne(ch chan PingWork) {
  var err error
  if len(pinger.workingApps) == 0 {
    pinger.workingApps, err = pinger.Store.ListApplications()
    if err != nil {
      log.Println("Failed to get app list")
      return
    }
  }

  app := pinger.workingApps[0]
  pinger.workingApps = pinger.workingApps[1:]

  _, belist, err := pinger.Store.DescribeApplication(app)

  if err != nil {
    log.Println("Failed to get backend list for", app)
    return
  }

  log.Println("Starting checks on", app)
  pw := PingWork{App: app}
  for be, _ := range belist {
    pw.Backends = append(pw.Backends, be)
  }

  ch <- pw
}

func (pinger *Pinger) Process(pw PingWork) error {
  for _, backend := range pw.Backends {
    alive := checkEndpoint(backend)

    status := "alive"
    if !alive {
      status = "dead"
    }

    log.Println("Backend [", pw.App, "]", backend, "is", status)

    if alive {
      err := pinger.Store.EnableBackend(pw.App, backend)
      if err != nil {
        return err
      }
    } else {
      err := pinger.Store.DisableBackend(pw.App, backend)
      if err != nil {
        return err
      }
    }
  }

  return nil
}

func (pinger *Pinger) Stop() error {
  pinger.q <- true
  return nil
}

// We don't care if it returns 2xx,3xx,4xx,5xx
// as long as it is alive
func checkEndpoint(endpoint string) bool {
  tr := &http.Transport{
    Dial: dialTimeout,
  }

  req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s", endpoint), nil)
  req.Close = true

  resp, err := tr.RoundTrip(req)
  if err == nil {
    defer resp.Body.Close()
  }

  if err != nil {
    return false
  }

  return true
}

func dialTimeout(network, addr string) (net.Conn, error) {
  timeout := time.Duration(2 * time.Second)
  return net.DialTimeout(network, addr, timeout)
}
