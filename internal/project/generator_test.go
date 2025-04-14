package project

import (
	"fmt"
	"github.com/inovacc/utils/v2/tree"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"log"
	"os"
	"path/filepath"
	"testing"
)

var afs = afero.NewOsFs()

func TestMain(t *testing.M) {
	_, err := afero.TempDir(afs, "", "cobra_")
	if err != nil {
		log.Fatalf("Could not create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll("cobra_"); err != nil {
			fmt.Printf("could not remove temp dir: %v\n", err)
		}
	}()

	os.Exit(t.Run())
}

func TestGenerateRoot(t *testing.T) {
	viper.SetDefault("projectName", "testApp")
	defer viper.Reset()

	project := NewProject(afs, []string{"myproject"})

	project.SetPkgName("github.com/acme/myproject")

	generator, err := NewProjectGenerator(afs, project, "apache2", "NAME HERE <EMAIL ADDRESS>")
	if err != nil {
		t.Fatal(err)
	}

	if err := generator.CreateProject(); err != nil {
		t.Fatalf("Error creating project: %v", err)
	}

	// Check LICENSE
	if !generator.noLicense {
		assertFileMatchesGolden(t, afs,
			filepath.Join(generator.project.AbsolutePath, "LICENSE"),
			"testdata/LICENSE.golden")
	}

	// Check main.go
	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "main.go"),
		func() string {
			if generator.noLicense {
				return "testdata/main_none.golden"
			}
			return "testdata/main.go.golden"
		}(),
	)

	// Check cmd/root.go
	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "cmd/root.go"),
		func() string {
			if generator.noLicense {
				return "testdata/root_none.golden"
			}
			return "testdata/root.golden"
		}(),
	)

	// Check config files
	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "internal/config/config.go"),
		"testdata/config.golden")

	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "internal/config/config_test.go"),
		"testdata/config_test.golden")

	// Check service
	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "internal/service/service.go"),
		"testdata/service.golden")
}

func TestGenerateSub(t *testing.T) {
	viper.SetDefault("projectName", "testApp")
	defer viper.Reset()

	project := NewProject(afs, []string{"service"})

	project.SetPkgName("github.com/acme/myproject")

	generator, err := NewProjectGenerator(afs, project, "apache2", "NAME HERE <EMAIL ADDRESS>")
	if err != nil {
		t.Fatal(err)
	}

	if err := generator.AddCommandProject(); err != nil {
		t.Fatalf("Error creating sub command: %v", err)
	}

	// Check subcommand
	assertFileMatchesGolden(t, afs,
		filepath.Join(generator.project.AbsolutePath, "service.go"),
		func() string {
			if generator.noLicense {
				return "testdata/add_command_none.golden"
			}
			return "testdata/add_command.golden"
		}(),
	)

	newTree := tree.NewTree(afs, filepath.Join(generator.project.AbsolutePath, ".."))

	if err := newTree.MakeTree(); err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	fmt.Println(newTree.ToString())
}

func TestModule(t *testing.T) {
	t.Log(getModImportPathV(afs))
}

func assertFileMatchesGolden(t *testing.T, fs afero.Fs, filePath string, goldenPath string) {
	t.Helper()

	exists, err := afero.Exists(fs, filePath)
	if err != nil || !exists {
		t.Fatalf("Expected file does not exist: %s", filePath)
	}

	actual, err := afero.ReadFile(fs, filePath)
	if err != nil {
		t.Fatalf("Error reading generated file: %s\n%v", filePath, err)
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Error reading golden file: %s\n%v", goldenPath, err)
	}

	if err := compareContent(actual, expected); err != nil {
		t.Fatalf("Mismatch for %s:\n%v", filePath, err)
	}
}
