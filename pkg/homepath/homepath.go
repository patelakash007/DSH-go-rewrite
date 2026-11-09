package homepath

import (
	"os"
	"path/filepath"
	"strings"
)

const HomeDirName = ".dsh"

const EnvName = "DSH_HOME"

func ExpandHome(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func Resolve(configured string, env map[string]string) (string, error) {
	var selected string
	switch {
	case strings.TrimSpace(configured) != "":
		selected = configured
	case strings.TrimSpace(env[EnvName]) != "":
		selected = env[EnvName]
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		selected = filepath.Join(home, HomeDirName)
	}
	expanded, err := ExpandHome(selected)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}

func Display(resolvedHome string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "$" + EnvName
	}
	def, _ := filepath.Abs(filepath.Join(home, HomeDirName))
	if filepath.Clean(resolvedHome) == def {
		return "~/" + HomeDirName
	}
	return "$" + EnvName
}
