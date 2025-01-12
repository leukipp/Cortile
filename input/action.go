package input

import (
	"os"
	"strings"
	"syscall"
	"time"

	"os/exec"

	"github.com/jezek/xgbutil/xevent"

	"github.com/leukipp/cortile/v2/common"
	"github.com/leukipp/cortile/v2/desktop"
	"github.com/leukipp/cortile/v2/store"
	"github.com/leukipp/cortile/v2/ui"

	log "github.com/sirupsen/logrus"
)

var (
	executeCallbacksFun []func(string, uint, uint) // Execute events callback functions
)

func Bind(tr *desktop.Tracker) {
	BindSignal(tr)
	BindMouse(tr)
	BindKeys(tr)
	BindTray(tr)
	BindDbus(tr)
	BindAddons(tr)
}

func ExecuteAction(action string, tr *desktop.Tracker, ws *desktop.Workspace) bool {
	success := false
	if len(action) == 0 || tr == nil || ws == nil {
		return false
	}

	log.Info("Execute action ", action, " [", ws.Name, "]")

	// Choose action command
	switch action {
	case "enable":
		success = EnableTiling(tr, ws)
	case "disable":
		success = DisableTiling(tr, ws)
	case "toggle":
		success = ToggleTiling(tr, ws)
	case "decoration":
		success = ToggleDecoration(tr, ws)
	case "restore":
		success = Restore(tr, ws)
	case "reset":
		success = Reset(tr, ws)
	case "cycle_next":
		success = CycleNext(tr, ws)
	case "cycle_previous":
		success = CyclePrevious(tr, ws)
	case "layout_vertical_left":
		success = VerticalLeftLayout(tr, ws)
	case "layout_vertical_right":
		success = VerticalRightLayout(tr, ws)
	case "layout_horizontal_top":
		success = HorizontalTopLayout(tr, ws)
	case "layout_horizontal_bottom":
		success = HorizontalBottomLayout(tr, ws)
	case "layout_maximized":
		success = MaximizedLayout(tr, ws)
	case "layout_fullscreen":
		success = FullscreenLayout(tr, ws)
	case "slave_increase":
		success = IncreaseSlave(tr, ws)
	case "slave_decrease":
		success = DecreaseSlave(tr, ws)
	case "master_increase":
		success = IncreaseMaster(tr, ws)
	case "master_decrease":
		success = DecreaseMaster(tr, ws)
	case "window_next":
		success = NextWindow(tr, ws)
	case "window_previous":
		success = PreviousWindow(tr, ws)
	case "window_up":
		success = DirectionWindow(ws, common.Up)
	case "window_down":
		success = DirectionWindow(ws, common.Down)
	case "window_left":
		success = DirectionWindow(ws, common.Left)
	case "window_right":
		success = DirectionWindow(ws, common.Right)
	case "position_next":
		success = NextPosition(tr, ws)
	case "position_previous":
		success = PreviousPosition(tr, ws)
	case "screen_next":
		success = NextScreen(tr, ws)
	case "screen_previous":
		success = PreviousScreen(tr, ws)
	case "master_make":
		success = MakeMaster(tr, ws)
	case "master_make_next":
		success = MakeMasterNext(tr, ws)
	case "master_make_previous":
		success = MakeMasterPrevious(tr, ws)
	case "proportion_increase":
		success = IncreaseProportion(tr, ws)
	case "proportion_decrease":
		success = DecreaseProportion(tr, ws)
	case "restart":
		success = Restart(tr)
	case "exit":
		success = Exit(tr)
	default:
		success = External(action)
	}
	time.AfterFunc(100*time.Millisecond, tr.Handlers.Reset)

	// Check success
	if !success {
		return false
	}

	// Execute callbacks
	executeCallbacks(action, ws.Location.Desktop, ws.Location.Screen)

	return true
}

func ExecuteActions(action string, tr *desktop.Tracker, mod string) bool {
	client := tr.ClientWorkspace(tr.ActiveClient())
	active := tr.ActiveWorkspace()

	// Use active client workspace as current
	if client != nil {
		active = client
	}

	// Execute actions per workspace
	results := []bool{}
	for _, ws := range tr.Workspaces {

		// Execute only on active screen
		if mod == "current" && ws.Location != active.Location {
			continue
		}

		// Execute only on active workspace
		if mod == "screens" && (ws.Location.Desktop != active.Location.Desktop) {
			continue
		}

		// Execute action and store results
		success := ExecuteAction(action, tr, ws)
		results = append(results, success)
	}

	return common.AllTrue(results)
}

func EnableTiling(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	ws.EnableTiling()
	tr.Update()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func DisableTiling(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.DisableTiling()
	tr.Restore(ws, store.Latest)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func ToggleTiling(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return EnableTiling(tr, ws)
	}
	return DisableTiling(tr, ws)
}

func EnableDecoration(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	mg := ws.ActiveLayout().GetManager()

	mg.EnableDecoration()
	tr.Update()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func DisableDecoration(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	mg := ws.ActiveLayout().GetManager()

	mg.DisableDecoration()
	tr.Update()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func ToggleDecoration(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	mg := ws.ActiveLayout().GetManager()
	if mg.DecorationDisabled() {
		return EnableDecoration(tr, ws)
	}
	return DisableDecoration(tr, ws)
}

func Restore(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.DisableTiling()
	tr.Restore(ws, store.Original)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func Reset(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ResetLayouts()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func CycleNext(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.CycleLayout(1)
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func CyclePrevious(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.CycleLayout(-1)
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func VerticalLeftLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "vertical-left" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func VerticalRightLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "vertical-right" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func HorizontalTopLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "horizontal-top" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func HorizontalBottomLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "horizontal-bottom" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func MaximizedLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "maximized" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func FullscreenLayout(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	for i, l := range ws.Layouts {
		if l.GetName() == "fullscreen" {
			ws.SetLayout(uint(i))
		}
	}
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func IncreaseSlave(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().IncreaseSlave()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func DecreaseSlave(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().DecreaseSlave()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func IncreaseMaster(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().IncreaseMaster()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func DecreaseMaster(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().DecreaseMaster()
	tr.Tile(ws)

	ui.ShowLayout(ws)
	ui.UpdateIcon(ws)

	return true
}

func NextWindow(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	c := ws.ActiveLayout().NextClient()
	if c == nil {
		return false
	}

	store.ActiveWindowSet(store.X, c.Window)

	return true
}

func PreviousWindow(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	c := ws.ActiveLayout().PreviousClient()
	if c == nil {
		return false
	}

	store.ActiveWindowSet(store.X, c.Window)

	return true
}

func DirectionWindow(ws *desktop.Workspace, d common.Direction) bool {
	if ws.TilingDisabled() {
		return false
	}

	c := ws.ActiveLayout().DirectionClient(d)
	if c == nil {
		return false
	}

	store.ActiveWindowSet(store.X, c.Window)

	return true
}

func NextPosition(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	c1 := ws.ActiveLayout().ActiveClient()
	if c1 == nil {
		return false
	}
	c2 := ws.ActiveLayout().NextClient()
	if c2 == nil {
		return false
	}

	ws.ActiveLayout().SwapClient(c1, c2)
	tr.Tile(ws)

	return true
}

func PreviousPosition(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	c1 := ws.ActiveLayout().ActiveClient()
	if c1 == nil {
		return false
	}
	c2 := ws.ActiveLayout().PreviousClient()
	if c2 == nil {
		return false
	}

	ws.ActiveLayout().SwapClient(c1, c2)
	tr.Tile(ws)

	return true
}

func NextScreen(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	c := tr.ActiveClient()
	if c == nil {
		return false
	}
	screen := int(c.Latest.Location.Screen) + 1
	if screen > int(store.Workplace.ScreenCount)-1 {
		return false
	}

	return c.MoveToScreen(uint32(screen))
}

func PreviousScreen(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	c := tr.ActiveClient()
	if c == nil {
		return false
	}
	screen := int(c.Latest.Location.Screen) - 1
	if screen < 0 {
		return false
	}

	return c.MoveToScreen(uint32(screen))
}

func MakeMaster(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	c := ws.ActiveLayout().ActiveClient()
	if c == nil {
		return false
	}

	ws.ActiveLayout().MakeMaster(c)
	tr.Tile(ws)

	return true
}

func MakeMasterNext(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	c := ws.ActiveLayout().NextClient()
	if c == nil {
		return false
	}

	ws.ActiveLayout().MakeMaster(c)
	tr.Tile(ws)

	return NextWindow(tr, ws)
}

func MakeMasterPrevious(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	c := ws.ActiveLayout().PreviousClient()
	if c == nil {
		return false
	}

	ws.ActiveLayout().MakeMaster(c)
	tr.Tile(ws)

	return PreviousWindow(tr, ws)
}

func IncreaseProportion(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().IncreaseProportion()
	tr.Tile(ws)

	return true
}

func DecreaseProportion(tr *desktop.Tracker, ws *desktop.Workspace) bool {
	if ws.TilingDisabled() {
		return false
	}
	ws.ActiveLayout().DecreaseProportion()
	tr.Tile(ws)

	return true
}

func Restart(tr *desktop.Tracker) bool {
	tr.Write()

	xevent.Detach(store.X, store.X.RootWin())

	for _, ws := range tr.Workspaces {
		if ws.TilingDisabled() {
			continue
		}
		ws.DisableTiling()
		tr.Restore(ws, store.Latest)
	}

	log.Info("Restart")

	// Communicate application exit
	Disconnect()

	// Restart application
	syscall.Exec(common.Process.Path, os.Args, os.Environ())

	return true
}

func Exit(tr *desktop.Tracker) bool {
	tr.Write()

	xevent.Detach(store.X, store.X.RootWin())

	for _, ws := range tr.Workspaces {
		if ws.TilingDisabled() {
			continue
		}
		ws.DisableTiling()
		tr.Restore(ws, store.Latest)
	}

	log.Info("Exit")

	// Communicate application exit
	Disconnect()

	// Exit application
	os.Exit(0)

	return true
}

func External(command string) bool {
	params := strings.Split(command, " ")

	if !common.HasFlag("enable-external-commands") {
		log.Warn("Executing external command \"", params[0], "\" disabled")
		return false
	}

	log.Info("Executing external command \"", params[0], " ", params[1:], "\"")

	// Execute external command
	cmd := exec.Command(params[0], params[1:]...)
	if err := cmd.Run(); err != nil {
		log.Error("External command failed: ", err)
		return false
	}

	return true
}

func OnExecute(fun func(string, uint, uint)) {
	executeCallbacksFun = append(executeCallbacksFun, fun)
}

func executeCallbacks(action string, desktop uint, screen uint) {
	log.Info("Execute event ", action)

	for _, fun := range executeCallbacksFun {
		fun(action, desktop, screen)
	}
}
