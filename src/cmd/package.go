package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/defenseunicorns/zarf/src/internal/k8s"
	"github.com/defenseunicorns/zarf/src/internal/message"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/pterm/pterm"
	"golang.org/x/exp/slices"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/internal/packager"
	"github.com/defenseunicorns/zarf/src/internal/packager/generator"
	"github.com/defenseunicorns/zarf/src/internal/packager/validate"
	"github.com/defenseunicorns/zarf/src/internal/utils"
	"github.com/mholt/archiver/v3"
	"github.com/spf13/cobra"
)

var insecureDeploy bool
var shasum string

var packageCmd = &cobra.Command{
	Use:     "package",
	Aliases: []string{"p"},
	Short:   "Zarf package commands for creating, deploying, and inspecting packages",
}

var packageCreateCmd = &cobra.Command{
	Use:     "create [DIRECTORY]",
	Aliases: []string{"c"},
	Args:    cobra.MaximumNArgs(1),
	Short:   "Use to create a Zarf package from a given directory or the current directory",
	Long: "Builds an archive of resources and dependencies defined by the 'zarf.yaml' in the active directory.\n" +
		"Private registries and repositories are accessed via credentials in your local '~/.docker/config.json' " +
		"and '~/.git-credentials'.\n",
	Run: func(cmd *cobra.Command, args []string) {

		var baseDir string

		// If a directory was provided, use that as the base directory
		if len(args) > 0 {
			baseDir = args[0]
		}

		var isCleanPathRegex = regexp.MustCompile(`^[a-zA-Z0-9\_\-\/\.\~\\:]+$`)
		if !isCleanPathRegex.MatchString(config.CommonOptions.CachePath) {
			message.Warnf("Invalid characters in Zarf cache path, defaulting to %s", config.ZarfDefaultCachePath)
			config.CommonOptions.CachePath = config.ZarfDefaultCachePath
		}

		packager.Create(baseDir)
	},
}

var packageDeployCmd = &cobra.Command{
	Use:     "deploy [PACKAGE]",
	Aliases: []string{"d"},
	Short:   "Use to deploy a Zarf package from a local file or URL (runs offline)",
	Long:    "Uses current kubecontext to deploy the packaged tarball onto a k8s cluster.",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var done func()
		packageName := choosePackage(args)
		config.DeployOptions.PackagePath, done = packager.HandleIfURL(packageName, shasum, insecureDeploy)
		defer done()
		packager.Deploy()
	},
}

var packageGenerateCmd = &cobra.Command{
	Use:     "generate NAME [--from data]...",
	Aliases: []string{"g"},
	Args:    cobra.ExactArgs(1),
	Short:   "Use to generate either an example package or a package from resources",
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]
		err := validate.ValidatePackageName(pkgName)
		if err != nil {
			message.Fatal(err, err.Error())
		}
		newPkg := types.ZarfPackage{
			Metadata: types.ZarfMetadata{
				Name: pkgName,
			},
			Kind: "ZarfPackageConfig",
		}
		if cmd.Flags().Changed("from") {
			if cmd.Flags().Changed("assume") {
				message.Warn("Zarf will assume all necessary parts of components because \"--assume\" has been set")
			}
			for _, componentSource := range config.GenerateOptions.From {
				message.Notef("Starting component generation from %s", componentSource)
				spinner := message.NewProgressSpinner("Deducing component type for %s", componentSource)
				result := generator.DeduceResourceType(componentSource)
				switch result {
				case "unknown path":
					spinner.Fatalf(errors.New("invalid path"), "The path %s is not valid or an empty folder", componentSource)
				case "unknown url":
					spinner.Fatalf(errors.New("invalid url"), "The url %s could not be reconciled into a component type", componentSource)
				case "unparsable":
					spinner.Fatalf(errors.New("invalid from arg"), "The value %s could not be reconciled into a url or path", componentSource)
				}
				spinner.Successf("%s's component from %s is a %s", pkgName, componentSource, result)

				newComponent := types.ZarfComponent{}

				switch result {
				case "localChart":
					newComponent = generator.GenLocalChart(componentSource)
				case "manifests":
					newComponent = generator.GenManifests(componentSource)
				case "localFiles":
					newComponent = generator.GenLocalFiles(componentSource)
				case "gitChart":
					newComponent = generator.GenGitChart(componentSource)
				case "helmRepoChart":
					newComponent = generator.GenHelmRepoChart(componentSource)
				case "remoteFile":
					newComponent = generator.GenRemoteFile(componentSource)
				}

				message.Info("Finding images...")

				// Gitrepo charts can never work reliably given message.fatal calls in underlying functions
				imageList := packager.GetImagesFromComponents([]types.ZarfComponent{newComponent}, "")

				// Put images from the FoundImages var into the component's images array, making sure to not create dupes
				for _, foundImages := range imageList {
					for realImage := range foundImages.RealImages {
						if !slices.Contains(newComponent.Images, realImage) {
							newComponent.Images = append(newComponent.Images, realImage)
						}
					}
					for maybeImage := range foundImages.MaybeImages {
						if !slices.Contains(newComponent.Images, maybeImage) {
							newComponent.Images = append(newComponent.Images, maybeImage)
						}
					}
				}

				message.SuccessF("Finished finding images. They may not be correct so please check!")

				newPkg.Components = append(newPkg.Components, newComponent)

			}
		} else {
			message.Fatal(errors.New("Unimplemented"), "Unimplemented")
		}
		message.Note("Component Generation Complete!")
		writeFile := true
		if _, err := os.Stat("zarf.yaml"); !config.CommonOptions.Confirm && err == nil {
			prompt := &survey.Confirm{
				Message: "A zarf.yaml already exists in your directory, would you like to overwite it?",
			}
			err := survey.AskOne(prompt, &writeFile)
			if err != nil {
				message.Fatal("Survey error", err.Error())
			}
		}
		if writeFile {
			spinner := message.NewProgressSpinner("Writing package file to %s", "zarf.yaml")
			err = utils.WriteYaml("zarf.yaml", newPkg, 0644)
			if err != nil {
				spinner.Fatalf(err, err.Error())
			}
			spinner.Successf("Package generated successfully! Package saved to %s", "zarf.yaml")
		} else {
			message.Error("write aborted", "Zarf package generation aborted.")
		}
	},
}

var packageInspectCmd = &cobra.Command{
	Use:     "inspect [PACKAGE]",
	Aliases: []string{"i"},
	Short:   "Lists the payload of a Zarf package (runs offline)",
	Long: "Lists the payload of a compiled package file (runs offline)\n" +
		"Unpacks the package tarball into a temp directory and displays the " +
		"contents of the archive.",
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		packageName := choosePackage(args)
		packager.Inspect(packageName)
	},
}

var packageListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List out all of the packages that have been deployed to the cluster",
	Run: func(cmd *cobra.Command, args []string) {
		// Get all the deployed packages
		deployedZarfPackages, err := k8s.GetDeployedZarfPackages()
		if err != nil {
			message.Fatalf(err, "Unable to get the packages deployed to the cluster")
		}

		// Populate a pterm table of all the deployed packages
		packageTable := pterm.TableData{
			{"     Package ", "Components"},
		}

		for _, pkg := range deployedZarfPackages {
			var components []string

			for _, component := range pkg.DeployedComponents {
				components = append(components, component.Name)
			}

			packageTable = append(packageTable, pterm.TableData{{
				fmt.Sprintf("     %s", pkg.Name),
				fmt.Sprintf("%v", components),
			}}...)
		}

		// Print out the table for the user
		_ = pterm.DefaultTable.WithHasHeader().WithData(packageTable).Render()
	},
}

var packageRemoveCmd = &cobra.Command{
	Use:     "remove {PACKAGE_NAME|PACKAGE_FILE}",
	Aliases: []string{"u"},
	Args:    cobra.ExactArgs(1),
	Short:   "Use to remove a Zarf package that has been deployed already",
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]
		isTarball := regexp.MustCompile(`.*zarf-package-.*\.tar\.zst$`).MatchString
		if isTarball(pkgName) {
			if utils.InvalidPath(pkgName) {
				message.Fatalf(nil, "Invalid tarball path provided")
			}

			tempPath, err := utils.MakeTempDir(config.CommonOptions.TempDirectory)
			if err != nil {
				message.Fatalf(err, "Unable to create tmpdir: %s", config.CommonOptions.TempDirectory)
			}
			defer os.RemoveAll(tempPath)

			if err := archiver.Unarchive(pkgName, tempPath); err != nil {
				message.Fatalf(err, "Unable to extract the package contents")
			}
			configPath := filepath.Join(tempPath, "zarf.yaml")

			var pkgConfig types.ZarfPackage

			if err := utils.ReadYaml(configPath, &pkgConfig); err != nil {
				message.Fatalf(err, "Unable to read zarf.yaml")
			}

			pkgName = pkgConfig.Metadata.Name
		}
		if err := packager.Remove(pkgName); err != nil {
			message.Fatalf(err, "Unable to remove the package with an error of: %#v", err)
		}
	},
}

func choosePackage(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	var path string
	prompt := &survey.Input{
		Message: "Choose or type the package file",
		Suggest: func(toComplete string) []string {
			files, _ := filepath.Glob(config.PackagePrefix + toComplete + "*.tar*")
			return files
		},
	}

	if err := survey.AskOne(prompt, &path, survey.WithValidator(survey.Required)); err != nil {
		message.Fatalf(nil, "Package path selection canceled: %s", err.Error())
	}

	return path
}

func init() {
	initViper()

	rootCmd.AddCommand(packageCmd)
	packageCmd.AddCommand(packageCreateCmd)
	packageCmd.AddCommand(packageDeployCmd)
	packageCmd.AddCommand(packageInspectCmd)
	packageCmd.AddCommand(packageGenerateCmd)
	packageCmd.AddCommand(packageRemoveCmd)
	packageCmd.AddCommand(packageListCmd)

	bindCreateFlags()
	bindDeployFlags()
	bindPackageGenerateFlags()
	bindInspectFlags()
	bindRemoveFlags()
}

func bindCreateFlags() {
	createFlags := packageCreateCmd.Flags()

	// Always require confirm flag (no viper)
	createFlags.BoolVar(&config.CommonOptions.Confirm, "confirm", false, "Confirm package creation without prompting")

	v.SetDefault(V_PKG_CREATE_SET, map[string]string{})
	v.SetDefault(V_PKG_CREATE_OUTPUT_DIR, "")
	v.SetDefault(V_PKG_CREATE_SKIP_SBOM, false)
	v.SetDefault(V_PKG_CREATE_INSECURE, false)

	createFlags.StringToStringVar(&config.CreateOptions.SetVariables, "set", v.GetStringMapString(V_PKG_CREATE_SET), "Specify package variables to set on the command line (KEY=value)")
	createFlags.StringVarP(&config.CreateOptions.OutputDirectory, "output-directory", "o", v.GetString(V_PKG_CREATE_OUTPUT_DIR), "Specify the output directory for the created Zarf package")
	createFlags.BoolVar(&config.CreateOptions.SkipSBOM, "skip-sbom", v.GetBool(V_PKG_CREATE_SKIP_SBOM), "Skip generating SBOM for this package")
	createFlags.BoolVar(&config.CreateOptions.Insecure, "insecure", v.GetBool(V_PKG_CREATE_INSECURE), "Allow insecure registry connections when pulling OCI images")
}

func bindDeployFlags() {
	deployFlags := packageDeployCmd.Flags()

	// Always require confirm flag (no viper)
	deployFlags.BoolVar(&config.CommonOptions.Confirm, "confirm", false, "Confirm package deployment without prompting")

	v.SetDefault(V_PKG_DEPLOY_SET, map[string]string{})
	v.SetDefault(V_PKG_DEPLOY_COMPONENTS, "")
	v.SetDefault(V_PKG_DEPLOY_INSECURE, false)
	v.SetDefault(V_PKG_DEPLOY_SHASUM, "")
	v.SetDefault(V_PKG_DEPLOY_SGET, "")

	deployFlags.StringToStringVar(&config.DeployOptions.SetVariables, "set", v.GetStringMapString(V_PKG_DEPLOY_SET), "Specify deployment variables to set on the command line (KEY=value)")
	deployFlags.StringVar(&config.DeployOptions.Components, "components", v.GetString(V_PKG_DEPLOY_COMPONENTS), "Comma-separated list of components to install.  Adding this flag will skip the init prompts for which components to install")
	deployFlags.BoolVar(&insecureDeploy, "insecure", v.GetBool(V_PKG_DEPLOY_INSECURE), "Skip shasum validation of remote package. Required if deploying a remote package and `--shasum` is not provided")
	deployFlags.StringVar(&shasum, "shasum", v.GetString(V_PKG_DEPLOY_SHASUM), "Shasum of the package to deploy. Required if deploying a remote package and `--insecure` is not provided")
	deployFlags.StringVar(&config.DeployOptions.SGetKeyPath, "sget", v.GetString(V_PKG_DEPLOY_SGET), "Path to public sget key file for remote packages signed via cosign")
}

func bindPackageGenerateFlags() {
	generateFlags := packageGenerateCmd.Flags()

	generateFlags.BoolVar(&config.CommonOptions.Confirm, "assume", false, "WARNING: Can have unexpected and usually incorrect results\nZarf will make assumptions about all aspects of package generation and will not ask the user for any input")
	generateFlags.StringArrayVarP(&config.GenerateOptions.From, "from", "f", []string{}, "The location of the resource to generate a package from")
	generateFlags.StringVarP(&config.GenerateOptions.Namespace, "namespace", "n", "", "The namespace for all generated components")
}

func bindInspectFlags() {
	inspectFlags := packageInspectCmd.Flags()
	inspectFlags.BoolVarP(&packager.ViewSBOM, "sbom", "s", false, "View SBOM contents while inspecting the package")
}

func bindRemoveFlags() {
	removeFlags := packageRemoveCmd.Flags()
	removeFlags.BoolVar(&config.CommonOptions.Confirm, "confirm", false, "REQUIRED. Confirm the removal action to prevent accidental deletions")
	removeFlags.StringVar(&config.DeployOptions.Components, "components", v.GetString(V_PKG_DEPLOY_COMPONENTS), "Comma-separated list of components to uninstall")
	_ = packageRemoveCmd.MarkFlagRequired("confirm")
}
