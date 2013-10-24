package knuckles

import (
	"testing"
	"time"
)

func Test_BackendInit(t *testing.T) {
	settings := BackendSettings{
		Endpoint:      "google.com:80",
		CheckUrl:      "http://google.com/",
		CheckInterval: 5000 * time.Millisecond,
		Updates:       make(chan BackendStatus, 1),
	}
	_, err := NewBackend("test", settings)
	if err != nil {
		t.Error(err)
	} else {
		t.Log("Backend init complete")
	}
}

func Test_BackendStop(t *testing.T) {
	settings := BackendSettings{
		Endpoint:      "google.com:80",
		CheckUrl:      "http://google.com/",
		CheckInterval: 5000 * time.Millisecond,
		Updates:       make(chan BackendStatus, 1),
	}
	be, err := NewBackend("test", settings)
	if err != nil {
		t.Error(err)
	}
	go be.Start()
	be.Stop()
	t.Log("Backend stopped")
}

func Test_BackendAlive(t *testing.T) {
	settings := BackendSettings{
		Endpoint:      "google.com:80",
		CheckUrl:      "http://google.com/",
		CheckInterval: 5000 * time.Millisecond,
		Updates:       make(chan BackendStatus, 1),
	}
	be, err := NewBackend("test", settings)
	if err != nil {
		t.Error(err)
	}

	go be.Start()

	var isAlive bool

	for i := 0; i < 3; i++ {
		select {
		case <-time.After(1 * time.Second):
			if isAlive = be.Alive; isAlive {
				t.Log("Backend is alive")
				be.Stop()
				return
			}
		}
	}

	if !isAlive {
		t.Error("Backend is dead")
	}
}
