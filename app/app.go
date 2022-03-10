package app

import (
	"fmt"
	"strconv"

	"redisterm/redisapi"
	"redisterm/tlog"
	"redisterm/ui"
)

// App app
type App struct {
	main *ui.MainView

	cfg    *config
	tree   *DBTree
	dbTree map[string]*DBTree
}

// NewApp new
func NewApp(config string) *App {
	a := &App{
		main:   ui.NewMainView(),
		dbTree: make(map[string]*DBTree),
		cfg:    newConfig(config),
	}
	a.init()
	return a
}

func (a *App) init() {
	a.main.GetConfig = a.GetConfig
	a.main.GetCmd().SetEnterHandler(a.onCmdLineEnter)
	tlog.SetLogger(a.main.GetOutput())
	a.main.RefreshOpLine(a.cfg.getDbNames(), a.Show)
	a.main.OnAdd = func(s ui.Setting) {
		if s.Name == "" {
			return
		}
		port, _ := strconv.Atoi(s.Port)
		conf := redisapi.RedisConfig{
			Name: s.Name,
			Host: s.Host,
			Port: port,
			Auth: s.Auth,
		}
		if a.cfg.update(conf) {
			a.main.GetOpLine().AddSelect(conf.Name)
		} else {
			a.main.RefreshOpLine(a.cfg.getDbNames(), a.Show)
		}
		if err := a.cfg.save(); err != nil {
			panic(err)
		}
	}
}

// Run run
func (a *App) Run() {
	a.main.GetOpLine().Select(0)

	if err := a.main.Run(); err != nil {
		panic(err)
	}

	for _, client := range a.dbTree {
		client.Close()
	}
}

// Show show
func (a *App) Show(index int) {
	config := a.cfg.getConfig(index)
	address := fmt.Sprintf("%v:%v", config.Host, config.Port)
	t, ok := a.dbTree[address]
	if !ok {
		tree := ui.NewTree("db")
		tree.GetRoot().SetReference(&Reference{
			Name: "db",
		})
		preview := ui.NewPreview()

		t = NewDBTree(tree, preview)
		t.ShowModalOK = a.main.ShowModalOK
		t.ShowModal = a.main.ShowModal
		data := NewData(address, config.Auth)
		if err := data.Connect(); err != nil {
			tlog.Log("[Show] %v", err)
		}
		t.SetData(config.Host, data)
		a.dbTree[address] = t
	}

	a.tree = t

	a.main.SetTree(a.tree.tree.TreeView)
	a.main.SetPreview(a.tree.preview.FlexBox())
	a.onCmdLineEnter("")
}

func (a *App) onCmdLineEnter(text string) {
	view := a.main.GetCmd()
	fmt.Fprintf(view, "[#00aa00]redis%v> [blue]", a.tree.data.index)
	fmt.Fprintln(view, text)
	fmt.Fprintf(view, "[white]")
	a.tree.data.Cmd(view, text)
}

func (a *App) GetConfig() ui.Setting {
	index := a.main.GetOpLine().GetSelect()
	config := a.cfg.getConfig(index)
	return ui.Setting{
		Name: config.Name,
		Host: config.Host,
		Port: strconv.Itoa(config.Port),
		Auth: config.Auth,
	}
}
