// Copyright © 2021 Steve Francia <spf@spf13.com>.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"github.com/inovacc/gofs-cli/internal/project"
	"github.com/inovacc/utils/v2/tree"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func init() {
	initCmd.SetOut(new(bytes.Buffer))
	initCmd.SetErr(new(bytes.Buffer))
}

var (
	initCmd = &cobra.Command{
		Use:     "init [path]",
		Aliases: []string{"initialize", "initialise", "create"},
		Short:   "Initialize a Cobra Application",
		Long: `Initialize (cobra-cli init) will create a new application, with a license
and the appropriate structure for a Cobra-based CLI application.

Cobra init must be run inside of a go module (please run "go mod init <MODNAME>" first)
`,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var comps []string
			var directive cobra.ShellCompDirective
			if len(args) == 0 {
				comps = cobra.AppendActiveHelp(comps, "Optionally specify the path of the go module to initialize")
				directive = cobra.ShellCompDirectiveDefault
			} else if len(args) == 1 {
				comps = cobra.AppendActiveHelp(comps, "This command does not take any more arguments (but may accept flags)")
				directive = cobra.ShellCompDirectiveNoFileComp
			} else {
				comps = cobra.AppendActiveHelp(comps, "ERROR: Too many arguments specified")
				directive = cobra.ShellCompDirectiveNoFileComp
			}
			return comps, directive
		},
		Run: func(cmd *cobra.Command, args []string) {
			afs := afero.NewOsFs()
			newProject := project.NewProject(args)

			projectGenerator, err := project.NewProjectGenerator(afs, newProject)
			cobra.CheckErr(err)

			cobra.CheckErr(projectGenerator.CreateProject())

			commands := []string{
				"gopkg.in/yaml.v3",
				"github.com/spf13/afero",
				"github.com/spf13/cobra",
				"github.com/spf13/viper",
				"github.com/google/uuid",
				"github.com/inovacc/logger",
				"go.uber.org/automaxprocs",
			}

			for _, command := range commands {
				_, err = project.GoCommand(command)
				cobra.CheckErr(err)
			}

			_, err = project.GoCommand("mod", "tidy", "-v")
			cobra.CheckErr(err)

			cmd.Printf("Your Cobra application is ready at\n%s\n", projectGenerator.GetProjectPath())

			newTree := tree.NewTree(afs, projectGenerator.GetProjectPath())

			cobra.CheckErr(newTree.MakeTree())

			cmd.Println(newTree.ToString())
		},
	}
)
