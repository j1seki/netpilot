package apps

import (
	"bufio"
	"bytes"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var desktopIcons = sync.OnceValue(func() map[string]string {
	icons := map[string]string{}
	for _, dir := range dataDirs() {
		files, _ := filepath.Glob(filepath.Join(dir, "applications", "*.desktop"))
		for _, f := range files {
			file, err := os.Open(f)
			if err != nil {
				continue
			}
			var executable, icon string
			inEntry := false
			scan := bufio.NewScanner(io.LimitReader(file, 128<<10))
			for scan.Scan() {
				line := strings.TrimSpace(scan.Text())
				if strings.HasPrefix(line, "[") {
					if inEntry {
						break
					}
					inEntry = line == "[Desktop Entry]"
				}
				if !inEntry {
					continue
				}
				if key, value, ok := strings.Cut(line, "="); ok {
					switch key {
					case "Exec":
						value = strings.TrimSpace(value)
						if strings.HasPrefix(value, `"`) {
							executable, _, _ = strings.Cut(value[1:], `"`)
						} else {
							executable, _, _ = strings.Cut(value, " ")
						}
					case "Icon":
						icon = value
					}
				}
			}
			file.Close()
			if icon == "" {
				continue
			}
			for _, key := range []string{executable, filepath.Base(executable), strings.TrimSuffix(filepath.Base(f), ".desktop")} {
				if key != "" && icons[key] == "" {
					icons[key] = icon
				}
			}
		}
	}
	return icons
})

func dataDirs() []string {
	home := os.Getenv("XDG_DATA_HOME")
	if home == "" {
		if user, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(user, ".local", "share")
		}
	}
	dirs := os.Getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	return append([]string{home}, filepath.SplitList(dirs)...)
}

func nativeIcon(executable string) []byte {
	icons := desktopIcons()
	base := filepath.Base(executable)
	name := icons[executable]
	if name == "" {
		name = icons[base]
	}
	if name == "" {
		name = base
	}
	if filepath.IsAbs(name) {
		return readDesktopIcon(name)
	}
	if filepath.Base(name) != name {
		return nil
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".png"), ".svg")
	for _, dir := range dataDirs() {
		for _, theme := range []string{"hicolor", "Adwaita", "breeze"} {
			for _, size := range []string{"32x32", "48x48", "64x64", "24x24", "128x128", "256x256", "scalable"} {
				path := filepath.Join(dir, "icons", theme, size, "apps", name)
				if b := readPNG(path + ".png"); b != nil {
					return b
				}
				if size == "scalable" {
					if b := readDesktopIcon(path + ".svg"); b != nil {
						return b
					}
				}
			}
		}
		if b := readPNG(filepath.Join(dir, "pixmaps", name+".png")); b != nil {
			return b
		}
	}
	return nil
}

func readDesktopIcon(path string) []byte {
	if strings.EqualFold(filepath.Ext(path), ".png") {
		return readPNG(path)
	}
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Size() <= 256<<10 {
			return rasterIcon(path)
		}
	}
	return nil
}

func readPNG(path string) []byte {
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() || st.Size() > 256<<10 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 256<<10))
	if err != nil {
		return nil
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(b)); err != nil || cfg.Width > 512 || cfg.Height > 512 {
		return nil
	}
	return b
}
