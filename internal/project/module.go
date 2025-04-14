package project

import (
	"fmt"
	"github.com/spf13/afero"
	"log"
	"path"
	"path/filepath"
	"strings"
)

func getModImportPathV(afs afero.Fs) (string, error) {
	mod, cd, err := parseModInfoV(afs)
	if err != nil {
		return "", err
	}
	str := path.Join(mod.Path, fileToURL(strings.TrimPrefix(cd.Dir, mod.Dir)))
	log.Println("Module import path:", str)
	return str, nil
}

func fileToURL(in string) string {
	i := strings.Split(in, string(filepath.Separator))
	return path.Join(i...)
}

type Mod struct {
	Dir         string
	ImportPath  string
	Name        string
	Target      string
	Root        string
	Module      CurDir
	Path        string
	GoMod       string
	GoVersion   string
	Match       []string
	Incomplete  bool
	Stale       bool
	StaleReason string
	GoFiles     []string
	Imports     []string
	Deps        []string
}

type CurDir struct {
	Path      string
	Main      bool
	Dir       string
	GoMod     string
	GoVersion string
}

func parseModInfoV(afs afero.Fs) (*Mod, *CurDir, error) {
	var (
		mod Mod
		dir CurDir
	)

	if err := modInfoJSON(&mod, "-m"); err != nil {
		return nil, nil, fmt.Errorf("cannot parse mod info -m, %v", err)
	}

	if mod.GoMod == "" {
		payload := []byte(fmt.Sprintf("module %s\n\ngo %s", mod.Name, mod.GoVersion))
		if err := afero.WriteFile(afs, "go.mod", payload, 0755); err != nil {
			return nil, nil, fmt.Errorf("cannot parse go.mod, %v", err)
		}

		if err := modInfoJSON(&mod, "-m"); err != nil {
			return nil, nil, fmt.Errorf("cannot parse mod info -m, %v", err)
		}
	}

	// Unsure why, but if no module is present Path is set to this string.
	if mod.Path == "command-line-arguments" {
		return nil, nil, fmt.Errorf("cannot create or read go module path")
	}

	// Unsure why, but if no module is present Path is set to this string.
	//if mod.Path == "command-line-arguments" {
	//	wd, err := os.Getwd()
	//	if err != nil {
	//		return nil, nil, fmt.Errorf("cannot get current working directory, %v", err)
	//	}
	//
	//	if _, err := os.Stat("go.mod"); err != nil {
	//		if _, err := GoCommand("mod", "init", path.Base(wd)); err != nil {
	//			return nil, nil, fmt.Errorf("cannot run [go mod init %s], %v", path.Base(wd), err)
	//		}
	//	}
	//}

	if err := modInfoJSON(&dir, "-e"); err != nil {
		return nil, nil, fmt.Errorf("cannot parse mod info -e, %v", err)
	}
	return &mod, &dir, nil
}
