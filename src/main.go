package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	components "github.com/MHNightCat/superfile/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/rkoesters/xdg/basedir"
	"github.com/urfave/cli/v2"
)

var HomeDir = basedir.Home
var SuperFileMainDir = basedir.ConfigHome + "/superfile"
var SuperFileCacheDir = basedir.CacheHome + "/superfile"
var SuperFileDataDir = basedir.DataHome + "/superfile"

const (
	currentVersion      string = "v1.0.2"
	latestVersionURL    string = "https://api.github.com/repos/MHNightCat/superfile/releases/latest"
	latestVersionGithub string = "github.com/MHNightCat/superfile/releases/latest"
	themeZip            string = "https://github.com/MHNightCat/superfile/raw/main/themeZip/1.0.2/theme.zip"
)

const (
	themeFolder      string = "/theme"
	lastCheckVersion string = "/lastCheckVersion"
	pinnedFile       string = "/pinned.json"
	configFile       string = "/config.toml"
	hotkeysFile      string = "/hotkeys.toml"
	toggleDotFile    string = "/toggleDotFile"
	themeZipName     string = "/theme.zip"
	logFile          string = "/superfile.log"
)

const (
	trashDirectory      string = "/Trash"
	trashDirectoryFiles string = "/Trash/files"
	trashDirectoryInfo  string = "/Trash/info"
)

func main() {
	output := termenv.NewOutput(os.Stdout)
	terminalBackgroundColor := output.BackgroundColor()
	app := &cli.App{
		Name:        "superfile",
		Version:     currentVersion,
		Description: "A Modern file manager with golang",
		ArgsUsage:   "[path]",
		Action: func(c *cli.Context) error {
			path := ""
			if c.Args().Present() {
				path = c.Args().First()
			}

			InitConfigFile()

			p := tea.NewProgram(components.InitialModel(path), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				output.SetBackgroundColor(terminalBackgroundColor)
				log.Fatalf("Alas, there's been an error: %v", err)
				os.Exit(1)
			}
			CheckForUpdates()
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln(err)
	}
}

func InitConfigFile() {
	config := struct {
		MainDir      string
		DataDir      string
		CacheDir     string
		PinnedFile   string
		ToggleFile   string
		LogFile      string
		ConfigFile   string
		HotkeysFile  string
		ThemeFolder  string
		ThemeZipName string
	}{
		MainDir:      SuperFileMainDir,
		DataDir:      SuperFileDataDir,
		CacheDir:     SuperFileCacheDir,
		PinnedFile:   pinnedFile,
		ToggleFile:   toggleDotFile,
		LogFile:      logFile,
		ConfigFile:   configFile,
		HotkeysFile:  hotkeysFile,
		ThemeFolder:  themeFolder,
		ThemeZipName: themeZipName,
	}

	// Create directories
	if err := createDirectories(
		config.MainDir, config.DataDir,
		config.CacheDir,
	); err != nil {
		log.Fatalln("Error creating directories:", err)
	}

	// Create trash directories
	if runtime.GOOS != "darwin" {
		if err := createDirectories(
			basedir.DataHome+trashDirectory,
			basedir.DataHome+trashDirectoryFiles,
			basedir.DataHome+trashDirectoryInfo,
		); err != nil {
			log.Fatalln("Error creating directories:", err)
		}
	}

	// Create files
	if err := createFiles(
		config.DataDir+config.PinnedFile,
		config.DataDir+config.ToggleFile,
		config.CacheDir+config.LogFile,
	); err != nil {
		log.Fatalln("Error creating files:", err)
	}

	// Write config file
	if err := writeConfigFile(config.MainDir+config.ConfigFile, configTomlString); err != nil {
		log.Fatalln("Error writing config file:", err)
	}

	if err := writeConfigFile(config.MainDir+config.HotkeysFile, hotkeysTomlString); err != nil {
		log.Fatalln("Error writing config file:", err)
	}

	// Download and install theme

	if err := downloadAndInstallTheme(config.MainDir, config.ThemeZipName, themeZip, config.ThemeFolder); err != nil {
		log.Fatalln("Error downloading theme:", err)
	}

}

// Helper functions
func createDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Directory doesn't exist, create it
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		} else if err != nil {
			// Some other error occurred while checking if the directory exists
			return fmt.Errorf("failed to check directory status %s: %w", dir, err)
		}
		// else: directory already exists
	}
	return nil
}

func createFiles(files ...string) error {
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			if err := os.WriteFile(file, nil, 0644); err != nil {
				return fmt.Errorf("failed to create file %s: %w", file, err)
			}
		}
	}
	return nil
}

func writeConfigFile(path, data string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			return fmt.Errorf("failed to write config file %s: %w", path, err)
		}
	}
	return nil
}

func downloadAndInstallTheme(dir, zipName, zipUrl, zipFolder string) error {
	if _, err := os.Stat(filepath.Join(dir, zipFolder)); os.IsNotExist(err) {

		err := DownloadFile(filepath.Join(SuperFileMainDir, zipName), zipUrl)
		if err != nil {
			return err
		}
		err = Unzip(filepath.Join(SuperFileMainDir, zipName), dir)
		if err != nil {
			return err
		} else {
			os.Remove(filepath.Join(SuperFileMainDir, zipName))
		}
	}
	return nil
}

func CheckForUpdates() {
	lastTime, err := ReadFromFile(SuperFileDataDir + lastCheckVersion)
	if err != nil && !os.IsNotExist(err) {
		fmt.Println("Error reading from file:", err)
		return
	}

	currentTime := time.Now()

	if lastTime.IsZero() || currentTime.Sub(lastTime) >= 24*time.Hour {
		resp, err := http.Get(latestVersionURL)
		if err != nil {
			fmt.Println("Error checking for updates:", err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		type GitHubRelease struct {
			TagName string `json:"tag_name"`
		}

		var release GitHubRelease
		if err := json.Unmarshal(body, &release); err != nil {
			return
		}

		if versionToNumber(release.TagName) > versionToNumber(currentVersion) {
			fmt.Printf("A new version %s is available.\n", release.TagName)
			fmt.Printf("Please update.\n┏\n\n        %s\n\n", latestVersionGithub)
			fmt.Printf("                                                               ┛\n")
		}

		timeStr := currentTime.Format(time.RFC3339)
		err = WriteToFile(SuperFileDataDir+lastCheckVersion, timeStr)
		if err != nil {
			log.Println("Error writing to file:", err)
			return
		}
	}
}

func versionToNumber(version string) int {
	version = strings.ReplaceAll(version, "v", "")
	version = strings.ReplaceAll(version, ".", "")

	num, _ := strconv.Atoi(version)
	return num
}

func ReadFromFile(filename string) (time.Time, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return time.Time{}, err
	}
	if len(content) == 0 {
		return time.Time{}, nil
	}
	lastTime, err := time.Parse(time.RFC3339, string(content))
	if err != nil {
		return time.Time{}, err
	}

	return lastTime, nil
}

func WriteToFile(filename, content string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}

func DownloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

const hotkeysTomlString string = `# Here is global, all global key cant conflicts with other hotkeys
quit = ["esc", "q"]

list_up = ["up", "k"]
list_down = ["down", "j"]

pinned_directory = ["ctrl+p", ""]

close_file_panel = ["ctrl+w", ""]
create_new_file_panel = ["ctrl+n", ""]

next_file_panel = ["tab", ""]
previous_file_panel = ["shift+left", ""]
focus_on_process_bar = ["p", ""]
focus_on_side_bar = ["b", ""]
focus_on_meta_data = ["m", ""]

change_panel_mode = ["v", ""]

file_panel_directory_create = ["f", ""]
file_panel_file_create = ["c", ""]
file_panel_item_rename = ["r", ""]
paste_item = ["ctrl+v", ""]
extract_file = ["ctrl+e", ""]
compress_file = ["ctrl+r", ""]

toggle_dot_file = ["ctrl+h", ""]

# These hotkeys do not conflict with any other keys (including global hotkey)
cancel = ["ctrl+c", "esc"]
confirm = ["enter", ""]

# Here is normal mode hotkey you can conflicts with other mode (cant conflicts with global hotkey)
delete_item = ["ctrl+d", ""]
select_item = ["enter", "l"]
parent_directory = ["h", "backspace"]
copy_single_item = ["ctrl+c", ""]
cut_single_item = ["ctrl+x", ""]
search_bar = ["ctrl+f", ""]

# Here is select mode hotkey you can conflicts with other mode (cant conflicts with global hotkey)
file_panel_select_mode_item_single_select = ["enter", "l"]
file_panel_select_mode_item_select_down = ["shift+down", "J"]
file_panel_select_mode_item_select_up = ["shift+up", "K"]
file_panel_select_mode_item_delete = ["ctrl+d", "delete"]
file_panel_select_mode_item_copy = ["ctrl+c", ""]
file_panel_select_mode_item_cut = ["ctrl+x", ""]
file_panel_select_all_item = ["ctrl+a", ""]
`

const configTomlString string = `# change your theme
theme = "gruvbox"

# useless for now
footer_panel_list = ["processes", "metadata", "clipboard"]


# ==========PLUGINS========== #

# Show more detailed metadata, please install exiftool before enabling this plugin!
metadata = false`
