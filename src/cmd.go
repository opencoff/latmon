// cmd.go -- cmd line commands

package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/opencoff/go-utils"
)

type Cmd interface {
	Name() string
	Desc() string
	Run(args []string) error
}

var cmds struct {
	sync.Mutex
	once sync.Once

	m  map[string]Cmd
	nm []string
	ab map[string]string
}

func registerCmd(c Cmd) {
	cmds.Lock()
	defer cmds.Unlock()

	if cmds.m == nil {
		cmds.m = make(map[string]Cmd)
	}

	nm := strings.ToLower(c.Name())
	if _, ok := cmds.m[nm]; ok {
		s := fmt.Sprintf("cmd %s already registered", nm)
		panic(s)
	}

	cmds.m[nm] = c
	cmds.nm = append(cmds.nm, nm)
}

func listCmds() []Cmd {
	cmds.Lock()
	defer cmds.Unlock()

	v := make([]Cmd, 0, len(cmds.m))
	for _, x := range cmds.m {
		v = append(v, x)
	}
	return v
}

func findCmd(nm string) (Cmd, error) {
	// make abbrev table
	cmds.once.Do(func() {
		cmds.ab = utils.Abbrev(cmds.nm)
	})

	nm = strings.ToLower(nm)

	cmds.Lock()
	defer cmds.Unlock()

	cn, ok := cmds.ab[nm]
	if !ok {
		return nil, fmt.Errorf("cmd: unknown command %s", nm)
	}

	cmd := cmds.m[cn]
	if cmd == nil {
		return nil, fmt.Errorf("cmd: empty command %s", nm)
	}
	return cmd, nil
}
