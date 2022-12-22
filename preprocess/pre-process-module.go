package preprocess

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/graph"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"os"
	"path/filepath"
	"strings"
)

func createModule(currentModuleName string, otherModuleNames []string, outPath string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	modulePath := filepath.Join(outPath, currentModuleName)
	terragruntOptions.Logger.Debugf("Creating module: %s", modulePath)

	if err := copyOriginalModuleFromWorkingDir(modulePath, terragruntOptions); err != nil {
		return err
	}

	if envName != nil {
		if err := copyEnvContents(modulePath, *envName, terragruntOptions); err != nil {
			return err
		}
	}

	// Since we copied all the contents over, we parse them again, and will modify the copies
	parsedTerraformFiles, err := parseAllTerraformFilesInDir(modulePath)
	if err != nil {
		return err
	}

	// Parse the .tfvars files as well so we can edit them if any variables are removed
	parsedTerraformVariableFiles, err := parseAllTerraformVariableFilesInDir(modulePath)
	if err != nil {
		return err
	}

	// We are going to modify the graph for each module, so clone it so we aren't modifying the original
	dependencyGraphClone := dependencyGraph.Clone()

	if err := processFiles(parsedTerraformFiles, modulePath, currentModuleName, otherModuleNames, envName, dependencyGraphClone, parsedTerraformVariableFiles, terragruntOptions); err != nil {
		return err
	}

	if err := writeFiles(parsedTerraformFiles, terragruntOptions); err != nil {
		return err
	}

	if err := writeFiles(parsedTerraformVariableFiles, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func copyOriginalModuleFromWorkingDir(modulePath string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Copying contents from %s to %s", terragruntOptions.WorkingDir, modulePath)
	return util.CopyFolderContentsWithFilterImpl(terragruntOptions.WorkingDir, modulePath, nil, false, preprocessorFileCopyFilter)
}

func copyEnvContents(modulePath string, envName string, terragruntOptions *options.TerragruntOptions) error {
	envPath := filepath.Join(terragruntOptions.WorkingDir, envsDirName, envName)
	terragruntOptions.Logger.Debugf("Copying contents from %s to %s", envPath, modulePath)
	return util.CopyFolderContentsWithFilterImpl(envPath, modulePath, nil, false, preprocessorFileCopyFilter)
}

// preprocessorFileCopyFilter is a filter that can be used with util.CopyFolderContentsWithFilter to exclude hidden
// files & folders and state files
func preprocessorFileCopyFilter(absolutePath string) bool {
	return !util.TerragruntExcludes(absolutePath) && !strings.HasSuffix(absolutePath, ".tfstate") && !strings.HasSuffix(absolutePath, ".tfstate.backup")
}

// TerraformFiles is a map from file path to the parsed HCL
type TerraformFiles map[string]*hclwrite.File

func parseAllTerraformFilesInDir(dir string) (TerraformFiles, error) {
	return parseAllTerraformFilesInDirThatMatchPattern(dir, "*.tf")
}

func parseAllTerraformVariableFilesInDir(dir string) (TerraformFiles, error) {
	return parseAllTerraformFilesInDirThatMatchPattern(dir, "*.tfvars")
}

func parseAllTerraformFilesInDirThatMatchPattern(dir string, pattern string) (TerraformFiles, error) {
	out := map[string]*hclwrite.File{}

	terraformFiles, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return out, errors.WithStackTrace(err)
	}

	for _, terraformFile := range terraformFiles {
		bytes, err := os.ReadFile(terraformFile)
		if err != nil {
			return out, errors.WithStackTrace(err)
		}

		parsedFile, diags := hclwrite.ParseConfig(bytes, terraformFile, hcl.InitialPos)
		if diags.HasErrors() {
			return out, errors.WithStackTrace(diags)
		}

		out[terraformFile] = parsedFile
	}

	return out, nil
}

func writeFiles(parsedTerraformFiles TerraformFiles, terragruntOptions *options.TerragruntOptions) error {
	for path, parsedFile := range parsedTerraformFiles {
		fileContents := parsedFile.Bytes()
		formattedFileContents := hclwrite.Format(fileContents)

		terragruntOptions.Logger.Debugf("Writing updated contents to %s", path)
		if err := util.WriteFileWithSamePermissions(path, path, formattedFileContents); err != nil {
			return err
		}
	}

	return nil
}
