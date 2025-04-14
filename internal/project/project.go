package project

import (
	"os"
	"path"
	"path/filepath"
)

type Project struct {
	Args         []string
	PkgName      string
	AbsolutePath string
	AppName      string
	CmdName      string
	Legal        *License
}

func NewProject(args []string) *Project {
	wd, _ := os.Getwd()

	if len(args) > 0 {
		if args[0] != "." {
			wd = filepath.Join(wd, args[0])
		}
	}

	return &Project{
		Args:         args,
		AbsolutePath: wd,
		AppName:      path.Base(wd),
		PkgName:      getModImportPath(wd),
		Legal:        &License{},
	}
}

func (p *Project) SetPkgName(value string) {
	p.PkgName = value
}

func (p *Project) SetAppName(value string) {
	p.AppName = value
}

func (p *Project) SetAbsolutePath(value string) {
	p.AbsolutePath = value
}
