package knuckles

import (
	"testing"
	"time"
)

func Test_FrontendStart(t *testing.T) {
	fe := NewFrontend("bazinga")
	go fe.Start()
	fe.Stop()
}

func Test_FrontendAdd(t *testing.T) {
	fe := NewFrontend("bazinga")
	go fe.Start()

	settings := BackendSettings{
		Endpoint:      "google.com:80",
		CheckUrl:      "http://google.com/",
		CheckInterval: 5000 * time.Millisecond,
		Updates:       make(chan BackendStatus, 1),
	}
	backend, _ := NewBackend("test", settings)
	err := fe.AddBackend(backend)
	if err != nil {
		t.Error(err)
	}

	err = fe.AddBackend(backend)
	if err == nil {
		t.Error("Should not allow same backend name twice")
	}
	fe.Stop()
}

func Test_FrontendPick(t *testing.T) {
	fe := NewFrontend("bazinga")
	go fe.Start()

	settings := BackendSettings{
		Endpoint:      "google.com:80",
		CheckUrl:      "http://google.com/",
		CheckInterval: 5000 * time.Millisecond,
		Updates:       fe.NotifyChan,
	}
	backend, _ := NewBackend("test", settings)
	fe.AddBackend(backend)
	time.Sleep(time.Second)
	pick, err := fe.PickBackend()

	if pick == nil {
		t.Error("Failed to return backend")
	}

	if err != nil {
		t.Error(err)
	}

	fe.Stop()
}
