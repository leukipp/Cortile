package desktop

import (
	"github.com/leukipp/cortile/common"
	"github.com/leukipp/cortile/layout"
	"github.com/leukipp/cortile/store"

	log "github.com/sirupsen/logrus"
)

type Workspace struct {
	Layouts         []Layout // List of vailable layouts
	TilingEnabled   bool     // Tiling is enabled or not
	ActiveLayoutNum uint     // Active layout index
}

func CreateWorkspaces() map[uint]*Workspace {
	workspaces := make(map[uint]*Workspace)

	for i := uint(0); i < common.DeskCount; i++ {

		// Create layouts for each workspace
		layouts := CreateLayouts(i)
		ws := Workspace{
			Layouts:       layouts,
			TilingEnabled: common.Config.TilingEnabled,
		}

		// Activate default layout
		for i, l := range layouts {
			if l.GetType() == common.Config.TilingLayout {
				ws.SetLayout(uint(i))
			}
		}

		workspaces[i] = &ws
	}

	return workspaces
}

func CreateLayouts(workspaceNum uint) []Layout {
	return []Layout{
		layout.CreateVerticalLayout(workspaceNum),
		layout.CreateHorizontalLayout(workspaceNum),
		layout.CreateFullscreenLayout(workspaceNum),
	}
}

func (ws *Workspace) SetLayout(layoutNum uint) {
	ws.ActiveLayoutNum = layoutNum
}

func (ws *Workspace) ActiveLayout() Layout {
	return ws.Layouts[ws.ActiveLayoutNum]
}

func (ws *Workspace) SwitchLayout() {
	ws.ActiveLayoutNum = (ws.ActiveLayoutNum + 1) % uint(len(ws.Layouts))
	ws.ActiveLayout().Do()
}

func (ws *Workspace) AddClient(c store.Client) {
	log.Debug("Add client [", c.Class, "]")

	// Add client to all layouts
	for _, l := range ws.Layouts {
		l.Add(c)
	}
}

func (ws *Workspace) RemoveClient(c store.Client) {
	log.Debug("Remove client [", c.Class, "]")

	// Remove client from all layouts
	for _, l := range ws.Layouts {
		l.Remove(c)
	}
}

func (ws *Workspace) IsMaster(c store.Client) bool {
	s := ws.ActiveLayout().GetManager()

	// Check if window is master
	for _, m := range s.Masters {
		if c.Win.Id == m.Win.Id {
			return true
		}
	}

	return false
}

func (ws *Workspace) Tile() {
	if ws.TilingEnabled {
		ws.ActiveLayout().Do()
	}
}

func (ws *Workspace) UnTile() {
	ws.ActiveLayout().Undo()
}