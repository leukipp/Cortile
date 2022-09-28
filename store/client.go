package store

import (
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
	"github.com/BurntSushi/xgbutil/motif"
	"github.com/BurntSushi/xgbutil/xprop"
	"github.com/BurntSushi/xgbutil/xrect"
	"github.com/BurntSushi/xgbutil/xwindow"

	"github.com/leukipp/cortile/common"

	log "github.com/sirupsen/logrus"
)

var UNKNOWN = "<UNKNOWN>"

type Client struct {
	Win         *xwindow.Window // X window object
	Info        Info
	CurrentProp Property // Properties that the client has at the moment
	SavedProp   Property // Properties that the client had before tiling
}

type Info struct {
	Class    string       // Client window application name
	Name     string       // Client window title name
	Desk     uint         // Desktop the client is currently in
	States   []string     // Client window states
	Hints    *motif.Hints // Client window hints
	IsGtkCSD bool
}

type Property struct {
	Geom xrect.Rect // Client rectangle geometry
	Deco bool       // Decoration active or not
}

func CreateClient(w xproto.Window) (c *Client) {
	win := xwindow.New(common.X, w)
	info := GetInfo(w)

	savedGeom, err := win.DecorGeometry()
	if err != nil {
		log.Info(err)
	}

	return &Client{
		Win:  win,
		Info: info,
		CurrentProp: Property{
			Geom: savedGeom,
			Deco: HasDecoration(w),
		},
		SavedProp: Property{
			Geom: savedGeom,
			Deco: HasDecoration(w),
		},
	}
}

func (c *Client) MoveResize(x, y, w, h int) {
	c.MoveResizeForTiler(x, y, w, h, true)
}

func (c *Client) MoveResizeForTiler(x, y, w, h int, isForTiling bool) {
	c.Unmaximize()

	dw, dh := c.DecorDimensions()

	if isForTiling && c.Info.IsGtkCSD {
		x, y, w, h = AdjustGtkWindowSizeForCSD(x, y, w, h, c.Win.Id)
	}

	// Move window
	err := c.Win.WMMoveResize(x, y, w-dw, h-dh)
	if err != nil {
		log.Warn("Error when moving window [", c.Info.Class, "]")
	}

	// Update stored dimensions
	c.Update()
}

func AdjustGtkWindowSizeForCSD(x, y, w, h int, win xproto.Window) (new_x, new_y, new_w, new_h int) {
	// Pre-Adjust the requested positioning and dimensions if the Window is a Gtk app with Client side decorations.
	extents, err := xprop.PropValNums(xprop.GetProperty(common.X, win, "_GTK_FRAME_EXTENTS"))
	if err == nil && len(extents) == 4 {
		new_x = x - int(extents[0])
		new_y = y - int(extents[2])
		new_w = w + int(extents[0]) + int(extents[1])
		new_h = h + (int(extents[2]) + int(extents[3]))
	} else {
		new_x, new_y, new_w, new_h = x, y, w, h
	}
	return
}

func (c *Client) DecorDimensions() (w int, h int) {

	// Inner dimension
	cGeom, err1 := xwindow.RawGeometry(common.X, xproto.Drawable(c.Win.Id))
	if err1 != nil {
		log.Warn(err1)
		return
	}

	// Outer dimension
	pGeom, err2 := c.Win.DecorGeometry()
	if err2 != nil {
		log.Warn(err2)
		return
	}

	w, h = pGeom.Width()-cGeom.Width(), pGeom.Height()-cGeom.Height()

	return
}

func (c *Client) Update() (success bool) {
	info := GetInfo(c.Win.Id)
	if info.Class == UNKNOWN {
		return false
	}

	// Set client infos
	c.Info = info

	// Update client geometry
	pGeom, err := c.Win.DecorGeometry()
	if err != nil {
		log.Warn(err)
		return false
	}
	c.CurrentProp.Geom = pGeom

	return true
}

func (c Client) Activate() {
	ewmh.ActiveWindowReq(common.X, c.Win.Id)
}

func (c Client) Unmaximize() {
	ewmh.WmStateReq(common.X, c.Win.Id, 0, "_NET_WM_STATE_MAXIMIZED_VERT")
	ewmh.WmStateReq(common.X, c.Win.Id, 0, "_NET_WM_STATE_MAXIMIZED_HORZ")
}

func (c Client) UnDecorate() {
	motif.WmHintsSet(common.X, c.Win.Id,
		&motif.Hints{
			Flags:      motif.HintDecorations,
			Decoration: motif.DecorationNone,
		})
}

func (c Client) Decorate() {
	if !c.SavedProp.Deco {
		return
	}

	motif.WmHintsSet(common.X, c.Win.Id,
		&motif.Hints{
			Flags:      motif.HintDecorations,
			Decoration: motif.DecorationAll,
		})
}

func (c Client) Restore() {
	c.Decorate()

	// Move window to stored position
	geom := c.SavedProp.Geom
	c.MoveResizeForTiler(geom.X(), geom.Y(), geom.Width(), geom.Height(), false)

	log.Info("Restoring window position x=", geom.X(), ", y=", geom.Y(), " [", c.Info.Class, "]")
}

func GetInfo(w xproto.Window) (info Info) {
	var err error
	var wmClass *icccm.WmClass

	var class string
	var name string
	var desk uint
	var states []string
	var hints *motif.Hints
	isGTKCSD := false

	// Class name
	wmClass, err = icccm.WmClassGet(common.X, w)
	if err != nil {
		log.Trace(err)
		class = UNKNOWN
	} else if wmClass != nil {
		class = wmClass.Class
	}

	// Windows title
	name, err = icccm.WmNameGet(common.X, w)
	if err != nil {
		log.Trace(err, " [", class, "]")
		name = UNKNOWN
	}

	// Window desktop
	desk, err = ewmh.WmDesktopGet(common.X, w)
	if err != nil {
		log.Trace(err, " [", class, "]")
		desk = math.MaxUint
	}

	// Window states
	states, err = ewmh.WmStateGet(common.X, w)
	if err != nil {
		log.Trace(err, " [", class, "]")
		states = []string{}
	}

	// Window hints
	hints, err = motif.WmHintsGet(common.X, w)
	if err != nil {
		hints = &motif.Hints{}
	}

	isGTKCSD = HasGtkFrameExtents(w)

	return Info{
		Class:    class,
		Name:     name,
		Desk:     desk,
		States:   states,
		Hints:    hints,
		IsGtkCSD: isGTKCSD,
	}
}

/*
_GTK_FRAME_EXTENTS is apparently the window property which tells you if a GTK app
is drawing it's own shadows and how big they are on each border
*/
func HasGtkFrameExtents(w xproto.Window) bool {
	extents, err := xprop.GetProperty(common.X, w, "_GTK_FRAME_EXTENTS")
	if err == nil && extents.Length == 4 {
		return true
	}
	return false
}

func HasDecoration(w xproto.Window) bool {
	info := GetInfo(w)
	return motif.Decor(info.Hints)
}

func IsMaximized(w xproto.Window) bool {
	info := GetInfo(w)
	if info.Class == UNKNOWN {
		return false
	}

	// Check maximized windows
	for _, state := range info.States {
		if strings.Contains(state, "_NET_WM_STATE_MAXIMIZED") {
			log.Info("Ignore maximized window", " [", info.Name, "]")
			return true
		}
	}

	return false
}

func IsInsideViewPort(w xproto.Window) bool {
	info := GetInfo(w)
	if info.Class == UNKNOWN {
		return false
	}

	// Window dimensions
	wGeom, err := xwindow.New(common.X, w).DecorGeometry()
	if err != nil {
		log.Warn(err)
		return false
	}

	// Viewport dimensions
	vRect := xrect.New(common.DesktopDimensions())

	// Substract viewport rectangle (r2) from window rectangle (r1)
	sRects := xrect.Subtract(wGeom, vRect)

	// If r1 does not overlap r2, then only one rectangle is returned which is equivalent to r1
	isOutsideViewport := false
	if len(sRects) == 1 {
		isOutsideViewport = reflect.DeepEqual(sRects[0], wGeom)
	}

	if isOutsideViewport {
		log.Info("Ignore window outside viewport [", info.Class, "]")
	}

	return !isOutsideViewport
}

func IsModal(info Info) bool {

	// Check model dialog windows
	for _, state := range info.States {
		if state == "_NET_WM_STATE_MODAL" {
			log.Info("Ignore modal window", " [", info.Name, "]")
			return true
		}
	}

	return false
}

func IsHidden(info Info) bool {

	// Check hidden windows
	for _, state := range info.States {
		if state == "_NET_WM_STATE_HIDDEN" {
			log.Info("Ignore hidden window", " [", info.Name, "]")
			return true
		}
	}

	return false
}

func IsFloating(info Info) bool {

	// Check floating state
	for _, state := range info.States {
		if state == "_NET_WM_STATE_ABOVE" {
			log.Info("Ignore floating window", " [", info.Name, "]")
			return true
		}
	}

	return false
}

func IsPinned(info Info) bool {

	// Check pinned windows
	if info.Desk > common.DeskCount {
		log.Info("Ignore pinned window [", info.Class, "]")
		return true
	}

	return false
}

func IsIgnored(info Info) bool {

	// Check ignored windows
	for _, s := range common.Config.WindowIgnore {
		conf_class := s[0]
		conf_name := s[1]

		reg_class := regexp.MustCompile(strings.ToLower(conf_class))
		reg_name := regexp.MustCompile(strings.ToLower(conf_name))

		// Ignore all windows with this class
		class_match := reg_class.MatchString(strings.ToLower(info.Class))

		// But allow the window with a special name
		name_match := conf_name != "" && reg_name.MatchString(strings.ToLower(info.Name))

		if class_match && !name_match {
			log.Info("Ignore window with ", strings.TrimSpace(strings.Join(s, " ")), " from config [", info.Name, "]")
			return true
		}
	}

	return false
}
