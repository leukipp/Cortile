package common

import (
	"fmt"
	"os"

	"encoding/json"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/fsnotify/fsnotify"

	log "github.com/sirupsen/logrus"
)

var (
	Config Configuration // Decoded config values
)

type Configuration struct {
	CacheWorkspaces   bool              `toml:"cache_workspaces"`    // Cache workspace properties (Tiling enablement, Current layout, proportions)
	CacheWindows      bool              `toml:"cache_windows"`       // Cache window properties ( Positions, Dimensions)
	TilingEnabled     bool              `toml:"tiling_enabled"`      // Tile windows on startup
	TilingLayout      string            `toml:"tiling_layout"`       // Initial tiling layout
	TilingCycle       []string          `toml:"tiling_cycle"`        // Cycle layout order
	TilingGui         int               `toml:"tiling_gui"`          // Time duration of gui
	TilingIcon        [][]string        `toml:"tiling_icon"`         // Menu entries of systray
	WindowIgnore      [][]string        `toml:"window_ignore"`       // Regex to ignore windows
	WindowMastersMax  int               `toml:"window_masters_max"`  // Maximum number of allowed masters
	WindowSlavesMax   int               `toml:"window_slaves_max"`   // Maximum number of allowed slaves
	WindowGapSize     int               `toml:"window_gap_size"`     // Gap size between windows
	WindowFocusDelay  int               `toml:"window_focus_delay"`  // Window focus delay when hovered
	WindowDecoration  bool              `toml:"window_decoration"`   // Show window decorations
	ProportionStep    float64           `toml:"proportion_step"`     // Master-slave area step size proportion
	ProportionMin     float64           `toml:"proportion_min"`      // Window size minimum proportion
	EdgeMargin        []int             `toml:"edge_margin"`         // Margin values of tiling area
	EdgeMarginPrimary []int             `toml:"edge_margin_primary"` // Margin values of primary tiling area
	EdgeCornerSize    int               `toml:"edge_corner_size"`    // Size of square defining edge corners
	EdgeCenterSize    int               `toml:"edge_center_size"`    // Length of rectangle defining edge centers
	Colors            map[string][]int  `toml:"colors"`              // List of color values for gui elements
	Keys              map[string]string `toml:"keys"`                // Event bindings for keyboard shortcuts
	Corners           map[string]string `toml:"corners"`             // Event bindings for hot-corner actions
	Systray           map[string]string `toml:"systray"`             // Event bindings for systray icon
}

/*
Partially for backward compatibility, partially to stop the config file getting huge,
for the options which are not commonly needed/wanted, set their default values.
*/
func SetConfigDefaults() {
	Config.CacheWindows = true
	Config.CacheWorkspaces = true
}

func InitConfig() {

	// Create config folder if not exists
	configFolderPath := filepath.Dir(Args.Config)
	if _, err := os.Stat(configFolderPath); os.IsNotExist(err) {
		os.MkdirAll(configFolderPath, 0755)
	}

	// Write default config if not exists
	if _, err := os.Stat(Args.Config); os.IsNotExist(err) {
		os.WriteFile(Args.Config, File.Toml, 0644)
	}

	// Read config file into memory
	readConfig(Args.Config, true)

	// Config file system watcher
	watchConfig(Args.Config)
}

func ConfigFolderPath(name string) string {

	// Obtain user config directory
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal("Error obtaining config directory: ", err)
	}

	return filepath.Join(userConfigDir, name)
}

func readConfig(configFilePath string, initial bool) {

	// Print runtime infos
	if initial {
		fmt.Print("BUILD")
		if HasReleaseInfos() {
			fmt.Printf(" [>>> %s v%s is available <<<]", Build.Name, Source.Releases[0].Name)
		}
		fmt.Printf(": \n  name: %s\n  target: %s\n  version: v%s-%s\n  date: %s\n  flags: %s\n\n", Build.Name, Build.Target, Build.Version, Build.Commit, Build.Date, Build.Flags)
		fmt.Printf("FILES: \n  log: %s\n  lock: %s\n  cache: %s\n  config: %s\n\n", Args.Log, Args.Lock, Args.Cache, configFilePath)
	}

	SetConfigDefaults()

	// Decode config file into struct
	_, err := toml.DecodeFile(configFilePath, &Config)
	if err != nil {
		if initial {
			log.Fatal("Error reading config file ", err)
		} else {
			log.Warn("Error updating config file ", err)
		}
	}

	// Print shortcut infos
	if initial {
		keys, _ := json.MarshalIndent(Config.Keys, "", "  ")
		corners, _ := json.MarshalIndent(Config.Corners, "", "  ")
		systray, _ := json.MarshalIndent(Config.Systray, "", "  ")

		fmt.Printf("KEYS: %s\n", RemoveChars(string(keys), []string{"{", "}", "\"", ","}))
		fmt.Printf("CORNERS: %s\n", RemoveChars(string(corners), []string{"{", "}", "\"", ","}))
		fmt.Printf("SYSTRAY: %s\n", RemoveChars(string(systray), []string{"{", "}", "\"", ","}))
	}
}

func watchConfig(configFilePath string) {

	// Init file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error(err)
	} else {
		watcher.Add(configFilePath)
	}

	// Listen for events
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					readConfig(configFilePath, false)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error(err)
			}
		}
	}()
}
