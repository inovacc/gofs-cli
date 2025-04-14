package project

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"
)

//go:embed tpl/*.tmpl
var templates embed.FS

var newProject = make(map[string]bool, 2)

var goBinary = "go"

// GoGet runs `go get -u <mod>` to fetch and update a module.
func GoGet(ctx context.Context, mod string) error {
	_, err := runGoCommand(ctx, []string{"get", "-u", mod})
	return err
}

// GoMod executes `go mod <args>` commands.
func GoMod(ctx context.Context, args ...string) error {
	allArgs := append([]string{"mod"}, args...)
	_, err := runGoCommand(ctx, allArgs)
	return err
}

// modInfoJSON unmarshal the output of `go list -json <args>` into the provided struct.
func modInfoJSON(v any, args ...string) error {
	allArgs := append([]string{"list", "-json"}, args...)
	out, err := runGoCommand(context.Background(), allArgs)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}

// runGoCommand constructs and executes a Go command, returning its output.
func runGoCommand(ctx context.Context, args []string) ([]byte, error) {
	cmdArgs := append([]string{goBinary}, args...)
	//log.Printf("Running command: %v\n", cmdArgs)

	out, err := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
	if err != nil {
		return nil, errors.New(string(out))
	}

	return out, nil
}

func compareContent(contentA, contentB []byte) error {
	if !bytes.Equal(ensureLF(contentA), ensureLF(contentB)) {
		return errors.New("byte slices differ")
	}
	return nil
}

func validateCmdName(args []string) string {
	var source string
	if len(args) > 0 {
		source = args[0]
	}

	var sb strings.Builder
	capitalize := false

	for i := 0; i < len(source); i++ {
		ch := source[i]
		if ch == '-' || ch == '_' {
			capitalize = true
			continue
		}
		if capitalize {
			sb.WriteByte(byte(unicode.ToUpper(rune(ch))))
			capitalize = false
		} else {
			sb.WriteByte(ch)
		}
	}

	if sb.Len() == 0 {
		return source
	}
	return sb.String()
}

// ensureLF converts any \r\n to \n
func ensureLF(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

func stat(afs afero.Fs, namePath string) bool {
	if _, err := afs.Stat(namePath); err != nil {
		return false
	}
	return true
}

//var (
//	ErrNoModuleFound     = errors.New("no module found in directory")
//	ErrRootFileMissing   = errors.New("root.go file not found in project directory")
//	ErrNoLicenseDetected = errors.New("no license comment detected in root.go")
//	ErrTemplateLoadFail  = errors.New("failed to load template file")
//	ErrGitInitFailed     = errors.New("git initialization failed")
//	ErrCommandAddFailed  = errors.New("failed to add new command to project")
//)

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
		if strings.Contains(info.Name(), name) {
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

type Project struct {
	Args         []string
	PkgName      string
	AbsolutePath string
	AppName      string
	CmdName      string
	Legal        *License
}

func NewProject(afs afero.Fs, args []string) *Project {
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
		PkgName:      getModImportPath(afs, wd),
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

type Command struct {
	CmdName          string
	CmdParent        string
	ExtractedLicense string
	*Project
}

type Content struct {
	Dirty            bool
	Name             string
	FilePath         string
	TemplateFilePath string
	TemplateContent  string
	Data             any
}

type Generator struct {
	afs         afero.Fs
	templates   embed.FS
	noLicense   bool
	project     *Project
	content     []Content
	userLicense string
	userAuthor  string
}

func NewProjectGenerator(fs afero.Fs, project *Project, userLicense, userAuthor string) (*Generator, error) {
	project.CmdName = validateCmdName(project.Args)
	project.Legal = newLicense(userLicense, userAuthor)

	return &Generator{
		noLicense:   project.Legal.Code == "none",
		afs:         fs,
		templates:   templates,
		project:     project,
		content:     []Content{},
		userLicense: userLicense,
		userAuthor:  userAuthor,
	}, nil
}

func (g *Generator) GetProjectPath() string {
	return g.project.AbsolutePath
}

func (g *Generator) CmdName() string {
	return g.project.CmdName
}

func (g *Generator) prepareModels() error {
	if !g.noLicense {
		fmt.Printf("* License: %s, Author: %s\n", g.userLicense, g.userAuthor)
		if err := g.getFileContentLicense(); err != nil {
			return err
		}
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
		fmt.Println("* Initializing git structure")
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

	fmt.Printf("* Creating project structure\n")

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
	return g.renderTemplate()
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

	if _, err := g.afs.Stat(".gitignore"); err != nil {
		if err := g.getFileContentIgnore(); err != nil {
			return err
		}
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

	data, err := g.templates.ReadFile(content.TemplateFilePath)
	if err != nil {
		return err
	}

	content.TemplateContent = string(data)
	content.Data = g.project.Legal
	content.Dirty = false
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

type License struct {
	Code      string
	Name      string
	Header    string
	Body      string
	Copyright string
}

func newLicense(userLicense, userAuthor string) *License {
	year := time.Now().Format("2006")

	switch userLicense {
	case "apache", "apache2":
		return &License{
			Code:      "apache_2",
			Name:      "Apache 2.0",
			Header:    getLicenseHeader(templates, "apache_2"),
			Body:      getLicenseBody(templates, "apache_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "mit":
		return &License{
			Code:      "mit",
			Name:      "MIT License",
			Header:    getLicenseHeader(templates, "mit"),
			Body:      getLicenseBody(templates, "mit"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "bsd3":
		return &License{
			Code:      "bsd_clause_3",
			Name:      "NewBSD",
			Header:    getLicenseHeader(templates, "bsd_clause_3"),
			Body:      getLicenseBody(templates, "bsd_clause_3"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "bsd2":
		return &License{
			Code:      "bsd_clause_2",
			Name:      "Simplified BSD License",
			Header:    getLicenseHeader(templates, "bsd_clause_2"),
			Body:      getLicenseBody(templates, "bsd_clause_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "gpl2":
		return &License{
			Code:      "gpl_2",
			Name:      "GNU General Public License 2.0",
			Header:    getLicenseHeader(templates, "gpl_2"),
			Body:      getLicenseBody(templates, "gpl_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "gpl3":
		return &License{
			Code:      "gpl_3",
			Name:      "GNU General Public License 3.0",
			Header:    getLicenseHeader(templates, "gpl_3"),
			Body:      getLicenseBody(templates, "gpl_3"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "lgpl":
		return &License{
			Code:      "lgpl",
			Name:      "GNU Lesser General Public License",
			Header:    getLicenseHeader(templates, "lgpl"),
			Body:      getLicenseBody(templates, "lgpl"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "agpl":
		return &License{
			Code:      "agpl",
			Name:      "GNU Affero General Public License",
			Header:    getLicenseHeader(templates, "agpl"),
			Body:      getLicenseBody(templates, "agpl"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	case "none":
		return &License{
			Code:      "none",
			Name:      "noLicense License",
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	default:
		return &License{
			Code:      "none",
			Name:      "noLicense License",
			Copyright: fmt.Sprintf("Copyright © %s %s", year, userAuthor),
		}
	}
}

func getLicenseHeader(templates embed.FS, code string) string {
	data, err := templates.ReadFile(fmt.Sprintf("tpl/header_%s.tmpl", code))
	if err != nil {
		return "No header license content"
	}
	return string(data)
}

func getLicenseBody(templates embed.FS, code string) string {
	data, err := templates.ReadFile(fmt.Sprintf("tpl/license_%s.tmpl", code))
	if err != nil {
		return "No license content"
	}
	return string(data)
}

func findLicenseAndRootGo(fs afero.Fs, root string) (string, string, error) {
	var licensePath, rootGoPath string

	root = filepath.Join(root, "..")

	err := afero.Walk(fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch info.Name() {
		case "LICENSE":
			licensePath = path
		case "root.go":
			rootGoPath = path
		}
		return nil
	})

	if err != nil {
		return "", "", err
	}

	if licensePath == "" || rootGoPath == "" {
		return "", "", fmt.Errorf("missing file(s): LICENSE=%v, root.go=%v", licensePath != "", rootGoPath != "")
	}

	return licensePath, rootGoPath, nil
}

func extractBlockCommentBeforePackage(fs afero.Fs, filePath string) (string, error) {
	content, err := afero.ReadFile(fs, filePath)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`(?s)/\*.*?\*/\s*package\s+cmd`)
	match := re.Find(content)
	if match == nil {
		return "", nil
	}

	block := regexp.MustCompile(`(?s)/\*.*?\*/`).Find(match)
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(block), "/*", ""), "*/", "")), nil
}

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
