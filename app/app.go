package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/liwnn/redisterm/config"
	"github.com/liwnn/redisterm/model"
	"github.com/liwnn/redisterm/redisapi"
	"github.com/liwnn/redisterm/tlog"
	"github.com/liwnn/redisterm/view"
)

// App app
type App struct {
	cfg  *config.Config
	main *view.MainView
	tree *DBTree

	dbTree map[string]*DBTree
}

// NewApp new
func NewApp(cfgFile string) *App {
	cfg, err := config.NewConfig(cfgFile)
	if err != nil {
		panic(err)
	}
	a := &App{
		main:   view.NewMainView(),
		dbTree: make(map[string]*DBTree),
		cfg:    cfg,
	}
	a.init()
	return a
}

func (a *App) init() {
	tlog.SetLogger(a.main.GetOutput())

	a.main.GetOpLine().SetEditClickFunc(func() {
		index := a.main.GetOpLine().GetSelect()
		config := a.cfg.GetConfig(index)
		setting := view.Setting{
			Name: config.Name,
			Host: config.Host,
			Port: strconv.Itoa(config.Port),
			Auth: config.Auth,
		}
		a.main.ShowConnSetting(setting, true)
		tlog.Log("[App] init Edit Click: %v", setting)
	})

	a.main.GetConnSetting().SetOKHandler(func(s view.Setting, edit bool) {
		a.main.HideConnSetting()
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
		fmt.Fprintln(a.main.GetOutput(), edit)
		if edit {
			lastIndex := a.main.GetOpLine().GetSelect()
			a.cfg.Update(conf, a.main.GetOpLine().GetSelect())
			a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
			a.main.GetOpLine().Select(lastIndex)
		} else {
			a.cfg.Add(conf)
			a.main.GetOpLine().AddSelect(conf.Name)
			a.main.GetOpLine().Select(a.main.GetOpLine().GetOptionCount() - 1)
		}
		if err := a.cfg.Save(); err != nil {
			panic(err)
		}
	})
	a.main.GetCmd().SetEnterHandler(a.onCmdLineEnter)
	a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
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
	config := a.cfg.GetConfig(index)
	address := fmt.Sprintf("%v:%v", config.Host, config.Port)
	t, ok := a.dbTree[address]
	if !ok {
		tree := view.NewTree("db")
		tree.GetRoot().SetReference(&Reference{
			Name: "db",
		})
		preview := view.NewPreview()

		t = NewDBTree(tree, preview)
		t.ShowModalOK = a.main.ShowModalOK
		t.ShowModal = a.main.ShowModal
		data := model.NewData(address, config.Auth)
		if err := data.Connect(); err != nil {
			tlog.Log("[Show] %v", err)
		}
		t.SetData(fmt.Sprintf("%s:%v", config.Host, config.Port), data)
		a.dbTree[address] = t
	}

	a.tree = t

	a.main.SetTree(a.tree.tree.TreeView)
	a.main.SetPreview(a.tree.preview.FlexBox())

	a.main.GetCmd().SetPromt(address, a.tree.data.Index())
}

func (a *App) onCmdLineEnter(text string) {
	args := strings.Fields(text)
	if len(args) == 0 {
		return
	}
	cmd := args[0]
	view := a.main.GetCmd()
	if err := a.tree.data.Cmd(view, cmd, args[1:]...); err != nil {
		fmt.Fprintln(view, err)
	} else {
		switch strings.ToUpper(cmd) {
		case "SELECT":
			index, err := strconv.Atoi(args[1])
			if err != nil {
				fmt.Fprintln(view, err)
			} else {
				view.SetIndex(index)
			}
		}
	}
}
