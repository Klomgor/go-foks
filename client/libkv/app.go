// Copyright (c) 2025 ne43, Inc.
// Licensed under the MIT License. See LICENSE in the project root for details.

package libkv

import (
	"sync"

	"github.com/foks-proj/go-foks/client/libclient"
	"github.com/foks-proj/go-foks/lib/core"
	proto "github.com/foks-proj/go-foks/proto/lib"
)

var AppID = libclient.AppID("kv")

func (k *App) Cleanup(m libclient.MetaContext) error { return nil }
func (k *App) ID() libclient.AppID                   { return AppID }

type App struct {
	sync.Mutex
	parent *libclient.UserContext
	user   *Minder
	teams  map[proto.FQTeam]*Minder
}

func NewApp(u *libclient.UserContext) *App {
	return &App{
		parent: u,
		teams:  make(map[proto.FQTeam]*Minder),
	}
}

func GetApp(u *libclient.UserContext) (*App, error) {
	ret := libclient.GetApp(u, AppID, NewApp)
	if ret == nil {
		return nil, core.InternalError("failed to get minder")
	}
	return ret, nil
}

func (a *App) getUser() *Minder {
	a.Lock()
	defer a.Unlock()
	if a.user == nil {
		a.user = NewMinder(a.parent)
	}
	return a.user
}

func (a *App) Minder(m MetaContext, actingAs *proto.FQTeamParsed) (*Minder, error) {
	if actingAs == nil {
		return a.getUser(), nil
	}
	fqp, err := a.parent.TeamMinder().ResolveAndReindex(m.MetaContext, *actingAs)
	if err != nil {
		return nil, err
	}
	a.Lock()
	defer a.Unlock()
	var ret *Minder
	if ret = a.teams[*fqp]; ret == nil {
		ret = NewMinder(a.parent)
		a.teams[*fqp] = ret
	}
	return ret, nil
}

func InitReq(m MetaContext, actingAs *proto.FQTeamParsed) (*Minder, error) {
	au := m.G().ActiveUser()
	if au == nil {
		return nil, core.NoActiveUserError{}
	}
	app, err := GetApp(au)
	if err != nil {
		return nil, err
	}
	ret, err := app.Minder(m, actingAs)
	if err != nil {
		return nil, err
	}
	return ret, nil
}
