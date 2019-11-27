package lib

import (
	"sync"
	"syscall"

	epoll "github.com/mailru/easygo/netpoll"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type Conmonmon struct {
	// ctrID to conmon
	conmons map[int]int
	mu      sync.RWMutex
	ep      *epoll.Epoll
	cb      func(string, syscall.Signal)
}

func NewConmonmon(cb func(string, syscall.Signal)) (*Conmonmon, error) {
	// initialize variables
	// create new epoll
	// start waiting on epoll
	config := epoll.EpollConfig{
		OnWaitError: epollOnError,
	}
	ep, err := epoll.EpollCreate(&config)
	if err != nil {
		return nil, err
	}

	cmm := Conmonmon{
		conmons: make(map[int]int),
		ep:      ep,
		cb:      cb,
	}
	return &cmm, nil
}

func epollOnError(err error) {
	logrus.Debugf(err.Error())
}

func (c *Conmonmon) AddConmon() error {
	// verify container state running
	// get conmon pid and container pid
	// get cgroup location of oom event
	// open oom file at that location
	// set callback for removing container after conmon ooms
	// return with epollctl
	fd := 0
	kcb := killCB{
		ctrID: "abcd",
		cb:    c.cb,
	}
	return c.ep.Add(fd, epoll.EPOLLIN, kcb.callback)
}

type killCB struct {
	ctrID string
	cb    func(string, syscall.Signal)
}

func (k *killCB) callback(events epoll.EpollEvent) {
	// TODO FIXME make sure this is as much as we're supposed to listen to
	if events|epoll.EPOLLIN != 1 {
		return
	}
	// write oom file
	// kill container
	k.cb(k.ctrID, unix.SIGKILL)
}

func (c *Conmonmon) RemoveConmon() {
	// verify conmon exists
	// remove from map
	// return with epollctl removal
}

func (c *Conmonmon) Restore() {
	// loop through containers
	// add container to map
	// register each container's conmon with the epoll instance
}
