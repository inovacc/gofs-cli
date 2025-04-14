package project

import (
	"embed"
	"fmt"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Licenses struct {
	licenses map[string]*License
}

type License struct {
	Code      string
	Name      string
	Header    string
	Body      string
	Copyright string
}

func newLicense() *Licenses {
	year := time.Now().Format("2006")

	l := &Licenses{}

	l.licenses = map[string]*License{
		"apache2": {
			Code:      "apache_2",
			Name:      "Apache 2.0",
			Header:    l.getLicenseHeader(templates, "apache_2"),
			Body:      l.getLicenseBody(templates, "apache_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"mit": {
			Code:      "mit",
			Name:      "MIT License",
			Header:    l.getLicenseHeader(templates, "mit"),
			Body:      l.getLicenseBody(templates, "mit"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"bsd3": {
			Code:      "bsd_clause_3",
			Name:      "NewBSD",
			Header:    l.getLicenseHeader(templates, "bsd_clause_3"),
			Body:      l.getLicenseBody(templates, "bsd_clause_3"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"bsd2": {
			Code:      "bsd_clause_2",
			Name:      "Simplified BSD License",
			Header:    l.getLicenseHeader(templates, "bsd_clause_2"),
			Body:      l.getLicenseBody(templates, "bsd_clause_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"gpl2": {
			Code:      "gpl_2",
			Name:      "GNU General Public License 2.0",
			Header:    l.getLicenseHeader(templates, "gpl_2"),
			Body:      l.getLicenseBody(templates, "gpl_2"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"gpl3": {
			Code:      "gpl_3",
			Name:      "GNU General Public License 3.0",
			Header:    l.getLicenseHeader(templates, "gpl_3"),
			Body:      l.getLicenseBody(templates, "gpl_3"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"lgpl": {
			Code:      "lgpl",
			Name:      "GNU Lesser General Public License",
			Header:    l.getLicenseHeader(templates, "lgpl"),
			Body:      l.getLicenseBody(templates, "lgpl"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"agpl": {
			Code:      "agpl",
			Name:      "GNU Affero General Public License",
			Header:    l.getLicenseHeader(templates, "agpl"),
			Body:      l.getLicenseBody(templates, "agpl"),
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
		"none": {
			Code:      "none",
			Name:      "noLicense License",
			Copyright: fmt.Sprintf("Copyright © %s %s", year, viper.GetString("author")),
		},
	}
	return l
}

func (l *Licenses) findLicense(name string) *License {
	item, ok := l.licenses[name]
	if !ok {
		return l.licenses["none"]
	}
	return item
}

func (l *Licenses) getLicenseHeader(templates embed.FS, code string) string {
	data, err := templates.ReadFile(fmt.Sprintf("tpl/header_%s.tmpl", code))
	if err != nil {
		return "No header license content"
	}
	return string(data)
}

func (l *Licenses) getLicenseBody(templates embed.FS, code string) string {
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
