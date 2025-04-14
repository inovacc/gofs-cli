package project

import (
	"embed"
	"errors"
	"fmt"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed tpl/*.tmpl
var templates embed.FS

var newProject = make(map[string]bool, 2)

func init() {
	if err := detectProjectStructure(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "initialization error: %v\n", err)
		os.Exit(1)
	}
}

func detectProjectStructure() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get working directory: %w", err)
	}

	check := func(name, file string) error {
		info, err := os.Stat(filepath.Join(wd, file))
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("error checking %s: %w", file, err)
		}
		if name != "git" || info.IsDir() {
			newProject[name] = true
		}
		return nil
	}

	if err := check("git", ".git"); err != nil {
		return err
	}
	return check("mod", "go.mod")
}

func getModImportPath(afs afero.Fs, wd string) string {
	mod, cd := parseModInfo(afs, wd)
	return path.Join(mod.Path, fileToURL(strings.TrimPrefix(cd.Dir, mod.Dir)))
}

func parseModInfo(afs afero.Fs, wd string) (*Mod, *CurDir) {
	var mod Mod
	var dir CurDir

	cobra.CheckErr(modInfoJSON(&mod, "-m"))

	if mod.Path == "command-line-arguments" {
		if _, err := os.Stat("go.mod"); err != nil {
			file, err := afs.Create("go.mod")
			cobra.CheckErr(err)

			_, err = file.WriteString(fmt.Sprintf("module %s\n\ngo %v", filepath.Base(wd), mod.GoVersion))
			cobra.CheckErr(err)

			cobra.CheckErr(modInfoJSON(&mod, "-m"))
		}
	}

	cobra.CheckErr(modInfoJSON(&dir, "-e"))
	return &mod, &dir
}

type Command struct {
	CmdName          string
	CmdParent        string
	ExtractedLicense string
	*Project
}

type Generator struct {
	afs       afero.Fs
	templates embed.FS
	noLicense bool
	project   *Project
	content   []Content
}

func NewProjectGenerator(fs afero.Fs, project *Project) (*Generator, error) {
	project.CmdName = validateCmdName(project.Args)

	licenses := newLicense()
	project.Legal = licenses.findLicense(viper.GetString("license"))

	return &Generator{
		noLicense: project.Legal.Code == "none",
		afs:       fs,
		templates: templates,
		project:   project,
		content:   []Content{},
	}, nil
}

type Content struct {
	Dirty            bool
	Name             string
	FilePath         string
	TemplateFilePath string
	TemplateContent  string
	Data             any
}

func (g *Generator) GetProjectPath() string {
	return g.project.AbsolutePath
}

func (g *Generator) CmdName() string {
	return g.project.CmdName
}

func (g *Generator) prepareModels() error {
	if err := g.getFileContentLicense(); err != nil {
		return err
	}

	if err := g.getFileContentMain(); err != nil {
		return err
	}

	if err := g.getFileContentRoot(); err != nil {
		return err
	}

	if err := g.getFileContentConfig(); err != nil {
		return err
	}

	if err := g.getFileContentService(); err != nil {
		return err
	}

	if err := g.getFileContentReadme(); err != nil {
		return err
	}

	if !newProject["git"] {
		if err := g.getFileContentIgnore(); err != nil {
			return err
		}

		if err := g.gitInit(); err != nil {
			return err
		}
	}

	return nil
}

// CreateProject sets up the Project structure and files.
func (g *Generator) CreateProject() error {
	if err := g.prepareModels(); err != nil {
		return err
	}

	if g.project.Legal == nil {
		return errors.New("no legal project")
	}

	// Ensure base directory exists
	if !stat(g.afs, g.project.AbsolutePath) {
		if err := g.afs.MkdirAll(g.project.AbsolutePath, 0754); err != nil {
			return err
		}
	}

	rootPath := filepath.Join(g.project.AbsolutePath, "cmd")
	if !stat(g.afs, rootPath) {
		if err := g.afs.MkdirAll(rootPath, 0751); err != nil {
			return err
		}
	}

	configPath := filepath.Join(g.project.AbsolutePath, "internal", "config")
	if !stat(g.afs, configPath) {
		if err := g.afs.MkdirAll(configPath, 0751); err != nil {
			return err
		}
	}

	servicePath := filepath.Join(g.project.AbsolutePath, "internal", "service")
	if !stat(g.afs, servicePath) {
		if err := g.afs.MkdirAll(servicePath, 0751); err != nil {
			return err
		}
	}

	//if err := g.goModInit(); err != nil {
	//	return err
	//}

	// if err := g.gitInit(); err != nil {
	// 	return err
	// }

	if err := g.renderTemplate(); err != nil {
		return err
	}

	return nil
}

// AddCommandProject sets up the Project structure and files for a new command.
func (g *Generator) AddCommandProject() error {
	// find LICENSE and root.go file in project
	_, rootGo, err := findLicenseAndRootGo(g.afs, g.project.AbsolutePath)
	if err != nil {
		return err
	}

	g.noLicense = false // set to false because, when new instance is created is set to true

	if !stat(g.afs, rootGo) {
		return fmt.Errorf("no root file found on: %s", rootGo)
	}

	g.project.AbsolutePath = filepath.Dir(rootGo)

	// Ensure base directory exists
	if !stat(g.afs, g.project.AbsolutePath) {
		if err := g.afs.MkdirAll(g.project.AbsolutePath, 0754); err != nil {
			return err
		}
	}

	if err := g.getFileContentSub(rootGo); err != nil {
		return err
	}

	if err := g.renderTemplate(); err != nil {
		return err
	}

	return nil
}

func (g *Generator) gitInit() error {
	if _, err := g.afs.Stat(".git"); err != nil {
		if err := exec.Command("git", "init").Run(); err != nil {
			return err
		}

		cmd := exec.Command("git", "branch", "-m", "main")
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run()
	}
	return nil
}

func (g *Generator) getFileContentMain() error {
	content := Content{
		Name:             "main",
		TemplateFilePath: "tpl/main.tmpl",
		FilePath:         fmt.Sprintf("%s/main.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	if g.noLicense {
		content.TemplateFilePath = "tpl/main_none.tmpl"
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project
	content.Dirty = false
	return nil
}

func (g *Generator) getFileContentLicense() error {
	content := Content{
		Name:             "license",
		FilePath:         fmt.Sprintf("%s/LICENSE", g.project.AbsolutePath),
		TemplateFilePath: fmt.Sprintf("tpl/license_%s.tmpl", g.project.Legal.Code),
		Dirty:            true,
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	if !g.noLicense {
		data, err := g.templates.ReadFile(content.TemplateFilePath)
		if err != nil {
			return err
		}

		content.TemplateContent = string(data)
		content.Data = g.project.Legal
		content.Dirty = false
	}

	return nil
}

func (g *Generator) getFileContentRoot() error {
	content := Content{
		Name:             "root",
		TemplateFilePath: "tpl/root.tmpl",
		FilePath:         fmt.Sprintf("%s/cmd/root.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	if g.noLicense {
		content.TemplateFilePath = "tpl/root_none.tmpl"
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project
	content.Dirty = false
	return nil
}

func (g *Generator) getFileContentConfig() error {
	content1 := Content{
		Name:             "config",
		TemplateFilePath: "tpl/config.tmpl",
		FilePath:         fmt.Sprintf("%s/internal/config/config.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	content2 := Content{
		Name:             "config_test",
		TemplateFilePath: "tpl/config_test.tmpl",
		FilePath:         fmt.Sprintf("%s/internal/config/config_test.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	content3 := Content{
		Name:             "config_custom",
		TemplateFilePath: "tpl/custom.tmpl",
		FilePath:         fmt.Sprintf("%s/internal/config/custom.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	defer func() {
		g.content = append(g.content, content1)
		g.content = append(g.content, content2)
		g.content = append(g.content, content3)
	}()

	data1, err := g.templates.ReadFile(content1.TemplateFilePath)
	if err != nil {
		return err
	}

	data2, err := g.templates.ReadFile(content2.TemplateFilePath)
	if err != nil {
		return err
	}

	data3, err := g.templates.ReadFile(content3.TemplateFilePath)
	if err != nil {
		return err
	}

	content1.TemplateContent = string(data1)
	content1.Data = g.project
	content1.Dirty = false

	content2.TemplateContent = string(data2)
	content2.Data = g.project
	content2.Dirty = false

	content3.TemplateContent = string(data3)
	content3.Data = g.project
	content3.Dirty = false
	return nil
}

func (g *Generator) getFileContentService() error {
	content := Content{
		Name:             "service",
		TemplateFilePath: "tpl/service.tmpl",
		FilePath:         fmt.Sprintf("%s/internal/service/service.go", g.project.AbsolutePath),
		Dirty:            true,
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project
	content.Dirty = false
	return nil
}

func (g *Generator) getFileContentIgnore() error {
	content := Content{
		Name:             "gitignore",
		TemplateFilePath: "tpl/gitignore.tmpl",
		FilePath:         fmt.Sprintf("%s/.gitignore", g.project.AbsolutePath),
		Dirty:            true,
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project
	content.Dirty = false
	return nil
}

func (g *Generator) getFileContentReadme() error {
	content := Content{
		Name:             "readme",
		TemplateFilePath: "tpl/readme.tmpl",
		FilePath:         fmt.Sprintf("%s/README.md", g.project.AbsolutePath),
		Dirty:            true,
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project
	content.Dirty = false
	return nil
}

func (g *Generator) getFileContentSub(rootGo string) error {
	content := Content{
		Name:             "add_command",
		FilePath:         fmt.Sprintf("%s/%s.go", g.project.AbsolutePath, g.project.AppName),
		TemplateFilePath: "tpl/add_command.tmpl",
		Dirty:            true,
	}

	comment, err := extractBlockCommentBeforePackage(g.afs, rootGo)
	if err != nil {
		return err
	}

	if comment == "" {
		g.noLicense = true
		content.TemplateFilePath = "tpl/add_command_none.tmpl"
	}

	defer func() {
		g.content = append(g.content, content)
	}()

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = Command{
		CmdParent:        "rootCmd",
		CmdName:          g.project.CmdName,
		Project:          g.project,
		ExtractedLicense: comment,
	}
	content.Dirty = false
	return nil
}

func (g *Generator) renderTemplate() error {
	for _, content := range g.content {
		if content.Dirty {
			continue
		}

		if err := renderFileContent(g.afs, content); err != nil {
			return err
		}
	}
	return nil
}

func renderFileContent(afs afero.Fs, content Content) error {
	file, err := afs.Create(content.FilePath)
	if err != nil {
		return err
	}
	defer func(mainFile afero.File) {
		if err := mainFile.Close(); err != nil {
			cobra.CheckErr(err)
		}
	}(file)

	tmpl, err := template.New(content.Name).Parse(content.TemplateContent)
	if err != nil {
		return err
	}

	return tmpl.Execute(file, content.Data)
}
