package api

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/project-flogo/cli/common"
	"github.com/project-flogo/cli/util"
)

const (
	fileEmbeddedAppGo string = "embeddedapp.go"
	fileShimSupportGo string = "shim_support.go"
	fileShimGo        string = "shim.go"
	fileBuildGo       string = "build.go"
	fileMakefile      string = "Makefile"
	dirShim           string = "shim"
)

type BuildOptions struct {
	OptimizeImports bool
	EmbedConfig     bool
	Shim            string
}

var fileSampleShimSupport = filepath.Join("examples", "engine", "shim", fileShimSupportGo)

func BuildProject(project common.AppProject, options BuildOptions) error {

	project.DepManager().AddLocalContribForBuild()

	useShim := options.Shim != ""

	err := createEmbeddedAppGoFile(project, options.EmbedConfig || useShim)
	if err != nil {
		return err
	}

	err = createShimSupportGoFile(project, useShim)
	if err != nil {
		return err
	}

	err = initMain(project, useShim)
	if err != nil {
		return err
	}

	if useShim {
		buildExist, err := prepareShim(project, options.Shim)
		if err != nil {
			return err
		}
		if buildExist {
			return nil
		}

	}

	err = util.ExecCmd(exec.Command("go", "build"), project.SrcDir())
	if err != nil {
		fmt.Println("Error in building", project.SrcDir())
		return err
	}

	// assume linux/darwin env or cross platform by default
	exe := "main"

	if GOOSENV == "windows" || (runtime.GOOS == "windows" && GOOSENV == "") {
		// env or cross platform is windows
		exe = "main.exe"
	}

	exePath := filepath.Join(project.SrcDir(), exe)

	if common.Verbose() {
		fmt.Println("Path to exe is ", exePath)
	}
	if _, err := os.Stat(exePath); err == nil {
		finalExePath := project.Executable()
		os.MkdirAll(filepath.Dir(finalExePath), os.ModePerm)
		err = os.Rename(exePath, project.Executable())

		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to build application, run with --verbose to see details")
	}

	return nil
}

func prepareShim(project common.AppProject, shim string) (bool, error) {

	buf, err := ioutil.ReadFile(filepath.Join(project.Dir(), fileFlogoJson))
	if err != nil {
		return false, err
	}

	flogoJSON := string(buf)

	descriptor, err := util.ParseAppDescriptor(flogoJSON)
	if err != nil {
		return false, err
	}

	err = registerImports(project, descriptor)
	if err != nil {
		return false, err
	}

	for _, trgCfg := range descriptor.Triggers {
		if trgCfg.Id == shim {

			ref := trgCfg.Ref

			if trgCfg.Ref == "" {
				found := false
				ref, found = GetAliasRef("flogo:trigger", trgCfg.Type)
				if !found {
					return false, fmt.Errorf("unable to determine ref for trigger: %s", trgCfg.Id)
				}
			}

			refImport, err := util.NewFlogoImportFromPath(ref)
			if err != nil {
				return false, err
			}

			path, err := project.GetPath(refImport)
			if err != nil {
				return false, err
			}
			var shimFilePath string

			shimFilePath = filepath.Join(path, dirShim, fileShimGo)

			if _, err := os.Stat(shimFilePath); err == nil {

				copyFile(shimFilePath, filepath.Join(project.SrcDir(), fileShimGo))

				// Check if this shim based trigger has a gobuild file. If the trigger has a gobuild
				// execute that file, otherwise check if there is a Makefile to execute
				goBuildFilePath := filepath.Join(path, dirShim, fileBuildGo)

				makefilePath := filepath.Join(shimFilePath, dirShim, fileMakefile)

				if _, err := os.Stat(goBuildFilePath); err == nil {
					fmt.Println("This trigger makes use of a go build file...")

					copyFile(goBuildFilePath, filepath.Join(project.SrcDir(), fileBuildGo))

					// Execute go run gobuild.go
					err = util.ExecCmd(exec.Command("go", "run", fileBuildGo), project.SrcDir())

					if err != nil {
						return false, err
					}
				} else if _, err := os.Stat(makefilePath); err == nil {
					//look for Makefile and execute it
					fmt.Println("Make File:", makefilePath)

					copyFile(makefilePath, filepath.Join(project.SrcDir(), fileMakefile))

					// Execute make
					cmd := exec.Command("make", "-C", project.SrcDir())
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					cmd.Env = util.ReplaceEnvValue(os.Environ(), "GOPATH", project.Dir())

					err = cmd.Run()
					if err != nil {
						return false, err
					}
				} else {
					return false, nil
				}
			}

			break
		}
	}

	return true, nil
}

func createShimSupportGoFile(project common.AppProject, create bool) error {

	shimSrcPath := filepath.Join(project.SrcDir(), fileShimSupportGo)

	if !create {
		if _, err := os.Stat(shimSrcPath); err == nil {
			os.Remove(shimSrcPath)
			if err != nil {
				return err
			}
		}
		//
		//shimSrcPath := filepath.Join(project.SrcDir(), fileShimSupportGo)
		//
		//if _, err := os.Stat(shimSrcPath); err == nil {
		//	os.Remove(shimSrcPath)
		//	if err != nil {
		//		return err
		//	}
		//}
		return nil
	}

	flogoCoreImport, err := util.NewFlogoImportFromPath(flogoCoreRepo)
	if err != nil {
		return err
	}

	corePath, err := project.GetPath(flogoCoreImport)
	if err != nil {
		return err
	}

	bytes, err := ioutil.ReadFile(filepath.Join(corePath, fileSampleShimSupport))
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(shimSrcPath, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

func createEmbeddedAppGoFile(project common.AppProject, create bool) error {

	embedSrcPath := filepath.Join(project.SrcDir(), fileEmbeddedAppGo)

	if !create {
		if _, err := os.Stat(embedSrcPath); err == nil {
			os.Remove(embedSrcPath)
			if err != nil {
				return err
			}
		}
		return nil
	}

	buf, err := ioutil.ReadFile(filepath.Join(project.Dir(), fileFlogoJson))
	if err != nil {
		return err
	}

	flogoJSON := string(buf)

	data := struct {
		FlogoJSON string
	}{
		flogoJSON,
	}

	f, err := os.Create(embedSrcPath)
	if err != nil {
		return err
	}
	RenderTemplate(f, tplEmbeddedAppGoFile, &data)
	f.Close()

	return nil
}

var tplEmbeddedAppGoFile = `// Do not change this file, it has been generated using flogo-cli
// If you change it and rebuild the application your changes might get lost
package main

// embedded flogo app descriptor file
const flogoJSON string = ` + "`{{.FlogoJSON}}`" + `

func init () {
	cfgJson = flogoJSON
}
`

func copyFile(srcFilePath, destFilePath string) error {

	bytes, err := ioutil.ReadFile(srcFilePath)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(destFilePath, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

func initMain(project common.AppProject, backupMain bool) error {

	//backup main if it exists
	mainGo := filepath.Join(project.SrcDir(), fileMainGo)
	mainGoBak := filepath.Join(project.SrcDir(), fileMainGo+".bak")

	if backupMain {
		if _, err := os.Stat(mainGo); err == nil {
			err = os.Rename(mainGo, mainGoBak)
			if err != nil {
				return err
			}
		} else if _, err := os.Stat(mainGoBak); err != nil {
			return fmt.Errorf("project corrupt, main missing")
		}
	} else {
		if _, err := os.Stat(mainGoBak); err == nil {
			err = os.Rename(mainGoBak, mainGo)
			if err != nil {
				return err
			}
		} else if _, err := os.Stat(mainGo); err != nil {
			return fmt.Errorf("project corrupt, main missing")
		}
	}

	return nil
}
