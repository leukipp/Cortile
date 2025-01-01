package store

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"encoding/json"
	"path/filepath"

	"github.com/jezek/xgb/xproto"

	"github.com/jezek/xgbutil/ewmh"
	"github.com/jezek/xgbutil/icccm"
	"github.com/jezek/xgbutil/motif"
	"github.com/jezek/xgbutil/xprop"
	"github.com/jezek/xgbutil/xrect"
	"github.com/jezek/xgbutil/xwindow"

	"github.com/leukipp/cortile/v2/common"

	log "github.com/sirupsen/logrus"
)

type Client struct {
	Window   *XWindow // X window object
	Original *Info    `json:"-"` // Original client window information
	Cached   *Info    `json:"-"` // Cached client window information
	Latest   *Info    // Latest client window information
	Locked   bool     // Internal client move/resize lock
}

type Info struct {
	Class      string     // Client window application name
	Name       string     // Client window title name
	Types      []string   // Client window types
	States     []string   // Client window states
	Location   Location   // Client window location
	Dimensions Dimensions // Client window dimensions
}

type Dimensions struct {
	Geometry   common.Geometry   // Client window geometry
	Hints      Hints             // Client window dimension hints
	Extents    ewmh.FrameExtents // Client window geometry extents
	AdjPos     bool              // Position adjustments on move/resize
	AdjSize    bool              // Size adjustments on move/resize
	AdjRestore bool              // Disable adjustments on restore
}

type Hints struct {
	Normal icccm.NormalHints // Client window geometry hints
	Motif  motif.Hints       // Client window decoration hints
}

const (
	Original uint8 = 1 // Flag to restore original info
	Cached   uint8 = 2 // Flag to restore cached info
	Latest   uint8 = 3 // Flag to restore latest info
)

func CreateClient(w xproto.Window) *Client {
	c := &Client{
		Window:   CreateXWindow(w),
		Original: GetInfo(w),
		Cached:   GetInfo(w),
		Latest:   GetInfo(w),
		Locked:   false,
	}

	// Read client from cache
	cached := c.Read()

	// Overwrite states, geometry and location
	c.Cached.States = cached.Latest.States
	c.Cached.Dimensions.Geometry = cached.Latest.Dimensions.Geometry
	c.Cached.Location.Screen = ScreenGet(cached.Latest.Dimensions.Geometry.Center())

	// Restore window position
	c.Restore(Cached)

	c.Latest.States = c.Cached.States
	c.Latest.Dimensions.Geometry = c.Cached.Dimensions.Geometry
	c.Latest.Location.Screen = c.Cached.Location.Screen

	return c
}

func (c *Client) Lock() {
	c.Locked = true
}

func (c *Client) UnLock() {
	c.Locked = false
}

func (c *Client) Limit(w, h int) bool {
	if !Compatible("icccm.SizeHintPMinSize") {
		return false
	}

	// Decoration extents
	ext := c.Latest.Dimensions.Extents
	dw, dh := ext.Left+ext.Right, ext.Top+ext.Bottom

	// Set window size limits
	nhints := c.Cached.Dimensions.Hints.Normal
	nhints.Flags |= icccm.SizeHintPMinSize
	nhints.MinWidth = uint(w - dw)
	nhints.MinHeight = uint(h - dh)
	icccm.WmNormalHintsSet(X, c.Window.Id, &nhints)

	return true
}

func (c *Client) UnLimit() bool {
	if !Compatible("icccm.SizeHintPMinSize") {
		return false
	}

	// Restore window size limits
	icccm.WmNormalHintsSet(X, c.Window.Id, &c.Cached.Dimensions.Hints.Normal)

	return true
}

func (c *Client) Decorate() bool {
	if _, exists := common.Config.Keys["decoration"]; !exists {
		return false
	}
	if motif.Decor(&c.Latest.Dimensions.Hints.Motif) || !motif.Decor(&c.Original.Dimensions.Hints.Motif) {
		return false
	}

	// Add window decorations
	mhints := c.Cached.Dimensions.Hints.Motif
	mhints.Flags |= motif.HintDecorations
	mhints.Decoration = motif.DecorationAll
	motif.WmHintsSet(X, c.Window.Id, &mhints)

	return true
}

func (c *Client) UnDecorate() bool {
	if _, exists := common.Config.Keys["decoration"]; !exists {
		return false
	}
	if !motif.Decor(&c.Latest.Dimensions.Hints.Motif) && motif.Decor(&c.Original.Dimensions.Hints.Motif) {
		return false
	}

	// Remove window decorations
	mhints := c.Cached.Dimensions.Hints.Motif
	mhints.Flags |= motif.HintDecorations
	mhints.Decoration = motif.DecorationNone
	motif.WmHintsSet(X, c.Window.Id, &mhints)

	return true
}

func (c *Client) Fullscreen() bool {
	if IsFullscreen(c.Latest) {
		return false
	}

	// Fullscreen window
	ewmh.WmStateReq(X, c.Window.Id, ewmh.StateAdd, "_NET_WM_STATE_FULLSCREEN")

	return true
}

func (c *Client) UnFullscreen() bool {
	if !IsFullscreen(c.Latest) {
		return false
	}

	// Unfullscreen window
	ewmh.WmStateReq(X, c.Window.Id, ewmh.StateRemove, "_NET_WM_STATE_FULLSCREEN")

	return true
}

func (c *Client) UnMaximize() bool {
	if !IsMaximized(c.Latest) {
		return false
	}

	// Unmaximize window
	ewmh.WmStateReq(X, c.Window.Id, ewmh.StateRemove, "_NET_WM_STATE_MAXIMIZED_VERT")
	ewmh.WmStateReq(X, c.Window.Id, ewmh.StateRemove, "_NET_WM_STATE_MAXIMIZED_HORZ")

	return true
}

func (c *Client) MoveToDesktop(desktop uint32) bool {
	if desktop == ^uint32(0) {
		ewmh.WmStateReq(X, c.Window.Id, ewmh.StateAdd, "_NET_WM_STATE_STICKY")
	}

	// Set client desktop
	ewmh.WmDesktopSet(X, c.Window.Id, uint(desktop))
	ewmh.ClientEvent(X, c.Window.Id, "_NET_WM_DESKTOP", int(desktop), int(2))

	return true
}

func (c *Client) MoveToScreen(screen uint32) bool {
	geom := Workplace.Displays.Screens[screen].Geometry

	// Calculate move to position
	_, _, w, h := c.OuterGeometry()
	x, y := common.MaxInt(geom.Center().X-w/2, geom.X+100), common.MaxInt(geom.Center().Y-h/2, geom.Y+100)

	// Move window and simulate tracker pointer press
	ewmh.MoveWindow(X, c.Window.Id, x, y)
	Pointer.Press()

	return true
}

func (c *Client) MoveWindow(x, y, w, h int) {
	if c.Locked {
		log.Info("Reject window move/resize [", c.Latest.Class, "]")

		// Remove lock
		c.UnLock()
		return
	}

	// Remove unwanted properties
	c.UnMaximize()
	c.UnFullscreen()

	// Calculate dimension offsets
	ext := c.Latest.Dimensions.Extents
	dx, dy, dw, dh := 0, 0, 0, 0

	if c.Latest.Dimensions.AdjPos {
		dx, dy = ext.Left, ext.Top
	}
	if c.Latest.Dimensions.AdjSize {
		dw, dh = ext.Left+ext.Right, ext.Top+ext.Bottom
	}

	// Move and/or resize window
	if w > 0 && h > 0 {
		ewmh.MoveresizeWindow(X, c.Window.Id, x+dx, y+dy, w-dw, h-dh)
	} else {
		ewmh.MoveWindow(X, c.Window.Id, x+dx, y+dy)
	}

	// Update stored dimensions
	c.Update()
}

func (c *Client) OuterGeometry() (x, y, w, h int) {

	// Outer window dimensions (x/y relative to workspace)
	oGeom, err := c.Window.Instance.DecorGeometry()
	if err != nil {
		return
	}

	// Inner window dimensions (x/y relative to outer window)
	iGeom, err := xwindow.RawGeometry(X, xproto.Drawable(c.Window.Id))
	if err != nil {
		return
	}

	// Reset inner window positions (some wm won't return x/y relative to outer window)
	if reflect.DeepEqual(oGeom, iGeom) {
		iGeom.XSet(0)
		iGeom.YSet(0)
	}

	// Decoration extents (l/r/t/b relative to outer window dimensions)
	ext := c.Latest.Dimensions.Extents
	dx, dy, dw, dh := ext.Left, ext.Top, ext.Left+ext.Right, ext.Top+ext.Bottom

	// Calculate outer geometry (including server and client decorations)
	x, y, w, h = oGeom.X()+iGeom.X()-dx, oGeom.Y()+iGeom.Y()-dy, iGeom.Width()+dw, iGeom.Height()+dh

	return
}

func (c *Client) Restore(flag uint8) {
	if flag == Latest {
		c.Update()
	}

	// Restore window states
	if flag == Cached {
		if IsSticky(c.Cached) {
			c.MoveToDesktop(^uint32(0))
		}
	}

	// Restore window sizes
	c.UnLimit()
	c.UnMaximize()
	c.UnFullscreen()

	// Restore window decorations
	if flag == Original {
		if common.Config.WindowDecoration {
			c.Decorate()
		} else {
			c.UnDecorate()
		}
		c.Update()
	}

	// Disable adjustments on restore
	if c.Latest.Dimensions.AdjRestore {
		c.Latest.Dimensions.AdjPos = false
		c.Latest.Dimensions.AdjSize = false
	}

	// Move window to restore position
	geom := c.Latest.Dimensions.Geometry
	switch flag {
	case Original:
		geom = c.Original.Dimensions.Geometry
	case Cached:
		geom = c.Cached.Dimensions.Geometry
	}
	c.MoveWindow(geom.X, geom.Y, geom.Width, geom.Height)
}

func (c *Client) Update() {
	info := GetInfo(c.Window.Id)
	if len(info.Class) == 0 {
		return
	}
	log.Debug("Update client info [", info.Class, "]")

	// Update client info
	c.Latest = info
}

func (c *Client) Write() {
	if common.CacheDisabled() {
		return
	}

	// Obtain cache object
	cache := c.Cache()

	// Parse client cache
	data, err := json.MarshalIndent(cache.Data, "", "  ")
	if err != nil {
		log.Warn("Error parsing client cache [", c.Latest.Class, "]")
		return
	}

	// Write client cache
	path := filepath.Join(cache.Folder, cache.Name)
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		log.Warn("Error writing client cache [", c.Latest.Class, "]")
		return
	}

	log.Trace("Write client cache data ", cache.Name, " [", c.Latest.Class, "]")
}

func (c *Client) Read() *Client {
	if common.CacheDisabled() {
		return c
	}

	// Obtain cache object
	cache := c.Cache()

	// Read client cache
	path := filepath.Join(cache.Folder, cache.Name)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		log.Info("No client cache found [", c.Latest.Class, "]")
		return c
	}

	// Parse client cache
	cached := &Client{}
	err = json.Unmarshal([]byte(data), &cached)
	if err != nil {
		log.Warn("Error reading client cache [", c.Latest.Class, "]")
		return c
	}

	log.Debug("Read client cache data ", cache.Name, " [", c.Latest.Class, "]")

	return cached
}

func (c *Client) Cache() common.Cache[*Client] {
	subfolder := c.Latest.Class
	filename := fmt.Sprintf("%s-%d", subfolder, c.Latest.Location.Desktop)

	// Create client cache folder
	folder := filepath.Join(common.Args.Cache, "workplaces", Workplace.Displays.Name, "clients", subfolder)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		os.MkdirAll(folder, 0755)
	}

	// Create client cache object
	cache := common.Cache[*Client]{
		Folder: folder,
		Name:   common.HashString(filename, 20) + ".json",
		Data:   c,
	}

	return cache
}

func (c *Client) IsNew() bool {
	created := time.UnixMilli(c.Window.Created)
	return time.Since(created) < 1000*time.Millisecond
}

func IsSpecial(info *Info) bool {

	// Check internal windows
	if info.Class == common.Build.Name {
		log.Info("Ignore internal window [", info.Class, "]")
		return true
	}

	// Check window types
	types := []string{
		"_NET_WM_WINDOW_TYPE_DOCK",
		"_NET_WM_WINDOW_TYPE_DESKTOP",
		"_NET_WM_WINDOW_TYPE_TOOLBAR",
		"_NET_WM_WINDOW_TYPE_UTILITY",
		"_NET_WM_WINDOW_TYPE_TOOLTIP",
		"_NET_WM_WINDOW_TYPE_SPLASH",
		"_NET_WM_WINDOW_TYPE_DIALOG",
		"_NET_WM_WINDOW_TYPE_COMBO",
		"_NET_WM_WINDOW_TYPE_NOTIFICATION",
		"_NET_WM_WINDOW_TYPE_DROPDOWN_MENU",
		"_NET_WM_WINDOW_TYPE_POPUP_MENU",
		"_NET_WM_WINDOW_TYPE_MENU",
		"_NET_WM_WINDOW_TYPE_DND",
	}
	for _, typ := range info.Types {
		if common.IsInList(typ, types) {
			log.Info("Ignore window with type ", typ, " [", info.Class, "]")
			return true
		}
	}

	// Check window states
	states := []string{
		"_NET_WM_STATE_HIDDEN",
		"_NET_WM_STATE_MODAL",
		"_NET_WM_STATE_ABOVE",
		"_NET_WM_STATE_BELOW",
		"_NET_WM_STATE_SKIP_PAGER",
		"_NET_WM_STATE_SKIP_TASKBAR",
	}
	for _, state := range info.States {
		if common.IsInList(state, states) {
			log.Info("Ignore window with state ", state, " [", info.Class, "]")
			return true
		}
	}

	return false
}

type ignoreSpec struct {
	class, name *regexp.Regexp
}

func (spec *ignoreSpec) String() string {
	return spec.class.String() + " " + spec.name.String()
}

var windowIgnoreList []ignoreSpec

func getWindowIgnoreList() []ignoreSpec {
	if len(windowIgnoreList) == 0 && len(common.Config.WindowIgnore) > 0 {
		for _, s := range common.Config.WindowIgnore {
			conf_class := s[0]
			conf_name := s[1]

			spec := ignoreSpec{
				class: regexp.MustCompile(strings.ToLower(conf_class)),
				name:  regexp.MustCompile(strings.ToLower(conf_name)),
			}
			windowIgnoreList = append(windowIgnoreList, spec)
		}
	}

	return windowIgnoreList
}

func IsIgnored(info *Info) bool {
	// Check invalid windows
	if len(info.Class) == 0 {
		log.Info("Ignore invalid window")
		return true
	}
	// Check ignored windows
	for _, spec := range getWindowIgnoreList() {
		// Ignore all windows with this class
		class_match := spec.class.MatchString(strings.ToLower(info.Class))

		// But allow the window with a special name
		name_match := spec.name.String() != "" && spec.name.MatchString(strings.ToLower(info.Name))

		if class_match && !name_match {
			log.Info("Ignore window with ", spec.String(), " from config [", info.Name, "]")
			return true
		}
	}

	return false
}

func IsFullscreen(info *Info) bool {
	return common.IsInList("_NET_WM_STATE_FULLSCREEN", info.States)
}

func IsMaximized(info *Info) bool {
	return common.IsInList("_NET_WM_STATE_MAXIMIZED_VERT", info.States) || common.IsInList("_NET_WM_STATE_MAXIMIZED_HORZ", info.States)
}

func IsMinimized(info *Info) bool {
	return common.IsInList("_NET_WM_STATE_HIDDEN", info.States)
}

func IsSticky(info *Info) bool {
	return common.IsInList("_NET_WM_STATE_STICKY", info.States)
}

func GetInfo(w xproto.Window) *Info {
	var err error

	var class string
	var name string
	var types []string
	var states []string
	var location Location
	var dimensions Dimensions

	// Window class (internal class name of the window)
	cls, err := icccm.WmClassGet(X, w)
	if err != nil {
		log.Trace("Error on request: ", err)
	} else if cls != nil {
		class = cls.Class
	}

	// Window name (title on top of the window)
	name, err = icccm.WmNameGet(X, w)
	if err != nil {
		name = class
	}

	// Window geometry (dimensions of the window)
	geom, err := CreateXWindow(w).Instance.DecorGeometry()
	if err != nil {
		geom = &xrect.XRect{}
	}

	// Window desktop and screen (window workspace location)
	desktop, err := ewmh.WmDesktopGet(X, w)
	sticky := desktop > Workplace.DesktopCount
	if err != nil || sticky {
		desktop = CurrentDesktopGet(X)
	}
	location = Location{
		Desktop: desktop,
		Screen:  ScreenGet(common.CreateGeometry(geom).Center()),
	}

	// Window types (types of the window)
	types, err = ewmh.WmWindowTypeGet(X, w)
	if err != nil {
		types = []string{}
	}

	// Window states (states of the window)
	states, err = ewmh.WmStateGet(X, w)
	if err != nil {
		states = []string{}
	}
	if sticky && !common.IsInList("_NET_WM_STATE_STICKY", states) {
		states = append(states, "_NET_WM_STATE_STICKY")
	}

	// Window normal hints (normal hints of the window)
	nhints, err := icccm.WmNormalHintsGet(X, w)
	if err != nil {
		nhints = &icccm.NormalHints{}
	}

	// Window motif hints (hints of the window)
	mhints, err := motif.WmHintsGet(X, w)
	if err != nil {
		mhints = &motif.Hints{}
	}

	// Window extents (server/client decorations of the window)
	extNet, _ := xprop.PropValNums(xprop.GetProperty(X, w, "_NET_FRAME_EXTENTS"))
	extGtk, _ := xprop.PropValNums(xprop.GetProperty(X, w, "_GTK_FRAME_EXTENTS"))

	ext := make([]uint, 4)
	for i, e := range extNet {
		ext[i] += e
	}
	for i, e := range extGtk {
		ext[i] -= e
	}

	// Window dimensions (geometry/extent information for move/resize)
	dimensions = Dimensions{
		Geometry: *common.CreateGeometry(geom),
		Hints: Hints{
			Normal: *nhints,
			Motif:  *mhints,
		},
		Extents: ewmh.FrameExtents{
			Left:   int(ext[0]),
			Right:  int(ext[1]),
			Top:    int(ext[2]),
			Bottom: int(ext[3]),
		},
		AdjPos:     (nhints.WinGravity > 1 && !common.AllZero(extNet)) || !common.AllZero(extGtk),
		AdjSize:    !common.AllZero(extNet) || !common.AllZero(extGtk),
		AdjRestore: !common.AllZero(extGtk),
	}

	return &Info{
		Class:      class,
		Name:       name,
		Types:      types,
		States:     states,
		Location:   location,
		Dimensions: dimensions,
	}
}
