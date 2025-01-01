package desktop

import (
	"fmt"
	"os"

	"encoding/json"
	"path/filepath"

	"github.com/leukipp/cortile/v2/common"
	"github.com/leukipp/cortile/v2/layout"
	"github.com/leukipp/cortile/v2/store"

	log "github.com/sirupsen/logrus"
)

type Workspace struct {
	Name     string         // Workspace location name
	Location store.Location // Desktop and screen location
	Layouts  []Layout       // List of available layouts
	Layout   uint           // Active layout index
	Tiling   bool           // Tiling is enabled
}

func CreateWorkspaces() map[store.Location]*Workspace {
	workspaces := make(map[store.Location]*Workspace)

	for desktop := uint(0); desktop < store.Workplace.DesktopCount; desktop++ {
		for screen := uint(0); screen < store.Workplace.ScreenCount; screen++ {
			location := store.Location{Desktop: desktop, Screen: screen}

			// Create layouts for each desktop and screen
			ws := &Workspace{
				Name:     fmt.Sprintf("workspace-%d-%d", location.Desktop, location.Screen),
				Location: location,
				Layouts:  CreateLayouts(location),
				Layout:   0,
				Tiling:   common.Config.TilingEnabled,
			}

			// Set default layout
			for i, l := range ws.Layouts {
				if l.GetName() == common.Config.TilingLayout {
					ws.SetLayout(uint(i))
				}
			}

			// Read workspace from cache
			cached := ws.Read()

			// Overwrite default layout, proportions, decoration and tiling state
			ws.SetLayout(cached.Layout)
			for _, l := range ws.Layouts {
				for _, cl := range cached.Layouts {
					if l.GetName() == cl.GetName() {
						mg, cmg := l.GetManager(), cl.GetManager()
						mg.Masters.Maximum = common.MinInt(cmg.Masters.Maximum, common.Config.WindowMastersMax)
						mg.Slaves.Maximum = common.MinInt(cmg.Slaves.Maximum, common.Config.WindowSlavesMax)
						mg.Proportions = cmg.Proportions
						mg.Decoration = cmg.Decoration
					}
				}
			}
			ws.Tiling = cached.Tiling

			// Map location to workspace
			workspaces[location] = ws
		}
	}

	return workspaces
}

func CreateLayouts(loc store.Location) []Layout {
	return []Layout{
		layout.CreateVerticalLeftLayout(loc),
		layout.CreateVerticalRightLayout(loc),
		layout.CreateHorizontalTopLayout(loc),
		layout.CreateHorizontalBottomLayout(loc),
		layout.CreateMaximizedLayout(loc),
		layout.CreateFullscreenLayout(loc),
	}
}

func (ws *Workspace) EnableTiling() {
	ws.Tiling = true
}

func (ws *Workspace) DisableTiling() {
	ws.Tiling = false
}

func (ws *Workspace) TilingEnabled() bool {
	if ws == nil {
		return false
	}
	return ws.Tiling
}

func (ws *Workspace) TilingDisabled() bool {
	if ws == nil {
		return true
	}
	return !ws.Tiling
}

func (ws *Workspace) ActiveLayout() Layout {
	return ws.Layouts[ws.Layout]
}

func (ws *Workspace) SetLayout(layout uint) {
	ws.Layout = layout
}

func (ws *Workspace) ResetLayouts() {

	// Reset layouts
	for _, l := range ws.Layouts {

		// Reset client decorations
		mg := l.GetManager()
		mg.Decoration = common.Config.WindowDecoration

		// Reset layout proportions
		l.Reset()
	}
}

func (ws *Workspace) CycleLayout(dir int) {
	cycle := common.Config.TilingCycle
	if len(cycle) == 0 {
		cycle = []string{"vertical-left", "vertical-right", "horizontal-top", "horizontal-bottom"}
	}

	// Map layout cycle names into layout indices
	indices := make([]int, len(cycle))
	for i, name := range cycle {
		for j, l := range ws.Layouts {
			if l.GetName() == name {
				indices[i] = j
			}
		}
	}

	// Obtain target layout index
	target := indices[0]
	if common.IsInList(ws.ActiveLayout().GetName(), cycle) {
		for i, name := range cycle {
			// Calculate next/previous layout index
			if ws.ActiveLayout().GetName() == name {
				index := (i + dir) % len(indices)
				target = indices[map[bool]int{true: index, false: len(indices) - 1}[index >= 0]]
			}
		}
	} else {
		// Restart if current layout is not in cycle list
		target = indices[map[bool]int{true: 0, false: len(indices) - 1}[dir >= 0]]
	}

	// Set active layout
	ws.SetLayout(uint(target))
}

func (ws *Workspace) AddClient(c *store.Client) {
	log.Info("Add client for each layout [", c.Latest.Class, "]")

	// Add client to all layouts
	for _, l := range ws.Layouts {
		l.AddClient(c)
	}
}

func (ws *Workspace) RemoveClient(c *store.Client) {
	log.Info("Remove client from each layout [", c.Latest.Class, "]")

	// Remove client from all layouts
	for _, l := range ws.Layouts {
		l.RemoveClient(c)
	}
}

func (ws *Workspace) VisibleClients() []*store.Client {
	al := ws.ActiveLayout()
	mg := al.GetManager()

	// Obtain visible clients
	clients := mg.Clients(store.Visible)
	if common.IsInList(al.GetName(), []string{"maximized", "fullscreen"}) {
		clients = mg.Visible(&store.Clients{Stacked: mg.Clients(store.Stacked), Maximum: 1})
	}

	return clients
}

func (ws *Workspace) Tile() {
	if ws.TilingDisabled() {
		return
	}
	mg := ws.ActiveLayout().GetManager()
	clients := mg.Clients(store.Stacked)

	// Set client decorations
	for _, c := range clients {
		if c == nil {
			continue
		}
		if mg.DecorationEnabled() {
			if c.Decorate() {
				c.Update()
			}
		} else {
			if c.UnDecorate() {
				c.Update()
			}
		}
	}

	// Apply active layout
	ws.ActiveLayout().Apply()
}

func (ws *Workspace) Restore(flag uint8) {
	mg := ws.ActiveLayout().GetManager()
	clients := mg.Clients(store.Stacked)

	log.Info("Untile ", len(clients), " windows [", ws.Name, "]")

	// Restore client dimensions
	for _, c := range clients {
		if c == nil {
			continue
		}
		c.Restore(flag)
	}
}

func (ws *Workspace) Write() {
	if common.CacheDisabled() || !common.Config.CacheWorkspaces {
		return
	}

	// Obtain cache object
	cache := ws.Cache()

	// Parse workspace cache
	data, err := json.MarshalIndent(cache.Data, "", "  ")
	if err != nil {
		log.Warn("Error parsing workspace cache [", ws.Name, "]")
		return
	}

	// Write workspace cache
	path := filepath.Join(cache.Folder, cache.Name)
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		log.Warn("Error writing workspace cache [", ws.Name, "]")
		return
	}

	log.Trace("Write workspace cache data ", cache.Name, " [", ws.Name, "]")
}

func (ws *Workspace) Read() *Workspace {
	if common.CacheDisabled() || !common.Config.CacheWorkspaces {
		return ws
	}

	// Obtain cache object
	cache := ws.Cache()

	// Read workspace cache
	path := filepath.Join(cache.Folder, cache.Name)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		log.Info("No workspace cache found [", ws.Name, "]")
		return ws
	}

	// Parse workspace cache
	cached := &Workspace{Layouts: CreateLayouts(ws.Location)}
	err = json.Unmarshal([]byte(data), &cached)
	if err != nil {
		log.Warn("Error reading workspace cache [", ws.Name, "]")
		return ws
	}

	log.Debug("Read workspace cache data ", cache.Name, " [", ws.Name, "]")

	return cached
}

func (ws *Workspace) Cache() common.Cache[*Workspace] {
	subfolder := fmt.Sprintf("workspace-%d", ws.Location.Desktop)
	filename := fmt.Sprintf("%s-%d", subfolder, ws.Location.Screen)

	// Create workspace cache folder
	folder := filepath.Join(common.Args.Cache, "workplaces", store.Workplace.Displays.Name, "workspaces", subfolder)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		os.MkdirAll(folder, 0755)
	}

	// Create workspace cache object
	cache := common.Cache[*Workspace]{
		Folder: folder,
		Name:   common.HashString(filename, 20) + ".json",
		Data:   ws,
	}

	return cache
}
