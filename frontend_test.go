package knuckles

import (
	"testing"
	"time"
)

func Test_FrontendStart(t *testing.T) {
	d := time.Duration(2 * time.Second)
	fe := NewFrontend("bazinga", d)
	go fe.Start()
	fe.Stop()
}

func Test_FrontendAdd(t *testing.T) {
	d := time.Duration(2 * time.Second)
	fe := NewFrontend("bazinga", d)
	err := fe.AddBackend("google", "google.com:80")
	if err != nil {
		t.Error(err)
	}
	go fe.Start()

	err = fe.AddBackend("google", "bogus:80")
	if err == nil {
		t.Error("Should not allow same backend name twice")
	}
	fe.Stop()
}

func Test_FrontendPick(t *testing.T) {
	d := time.Duration(2 * time.Second)
	fe := NewFrontend("bazinga", d)
	fe.AddBackend("bogus", "bogus:80")
	fe.AddBackend("google", "google.com:80")
	go fe.Start()

	time.Sleep(time.Second)

	_, err := fe.PickBackend()

	if err != nil {
		t.Error(err)
	}

	fe.Stop()
}
