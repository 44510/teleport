// Copyright 2021 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

// To run one of these pipelines locally:
// # Drone requires certain variables to be set
// export DRONE_REMOTE_URL="https://github.com/gravitational/teleport"
// export DRONE_SOURCE_BRANCH="$(git branch --show-current)"
// # `drone exec` does not support `exec` or `kubernetes` pipelines
// sed -i '' 's/type\: kubernetes/type\: docker/' .drone.yml && sed -i '' 's/type\: exec/type\: docker/' .drone.yml
// # Drone has a bug where "workspace" is appended to "/drone/src". This fixes that by updating references
// sed -i '' 's~/go/~/drone/src/go/~g' .drone.yml
// # Pull the current branch instead of v10
// sed -i '' "s~git checkout -qf \"\$(cat '/go/vars/full-version/v10')\"~git checkout -qf \"${DRONE_SOURCE_BRANCH}\"~" .drone.yml
// # `drone exec` does not properly map the workspace path. This creates a volume to be shared between steps
// #  at the correct path
// DOCKER_VOLUME_NAME="go"
// docker volume create "$DOCKER_VOLUME_NAME"
// drone exec --trusted --pipeline teleport-container-images-current-version-cron --clone=false --volume "${DOCKER_VOLUME_NAME}:/go"
// # Cleanup
// docker volume rm "$DOCKER_VOLUME_NAME"

import (
	"fmt"
	"path"
	"strings"
)

// If you are working on a PR/testing changes to this file you should configure the following for Drone testing:
// 1. Publish the branch you're working on
// 2. Set `PRBranch` to the name of the branch in (1)
// 3. Set `ConfigureForPRTestingOnly` to true
// 4. Create a public and private ECR, Quay repos for "teleport", "teleport-ent", "teleport-operator", "teleport-lab"
// 5. Set `TestingQuayRegistryOrg` and `TestingECRRegistryOrg` to the org name(s) used in (4)
// 6. Set the `ECRTestingDomain` to the domain used for the private ECR repos
// 7. Create two separate IAM users, each with full access to either the public ECR repo OR the private ECR repo
// 8. Create a Quay "robot account" with write permissions for the created Quay repos
// 9. Set the Drone secrets for the secret names listed in "GetContainerRepos" to the credentials in (7, 8), prefixed by the value of `TestingSecretPrefix`
//
// On each commit, after running `make dronegen``, run the following commands and resign the file:
// # Pull the current branch instead of v10 so the appropriate dockerfile gets loaded
// sed -i '' "s~git checkout -qf \"\$(cat '/go/vars/full-version/v10')\"~git checkout -qf \"${DRONE_SOURCE_BRANCH}\"~" .drone.yml
//
// When finishing up your PR check the following:
// * The testing secrets added to Drone have been removed
// * `ConfigureForPRTestingOnly` has been set to false, and `make dronegen` has been reran afterwords

const (
	ConfigureForPRTestingOnly bool   = true
	TestingSecretPrefix       string = "TEST_"
	TestingQuayRegistryOrg    string = "fred_heinecke"
	TestingECRRegistryOrg     string = "u8j2q1d9"
	TestingEcrRegion          string = "us-east-2"
	PRBranch                  string = "fred/arm-container-images"
	ECRTestingDomain          string = "278576220453.dkr.ecr.us-east-2.amazonaws.com"
)

const (
	ProductionRegistryOrg string = "gravitational"
	PublicEcrRegion       string = "us-east-1"
	StagingEcrRegion      string = "us-west-2"

	LocalRegistry string = "drone-docker-registry:5000"
)

func buildContainerImagePipelines() []pipeline {
	// These need to be updated on each major release.
	latestMajorVersions := []string{"v10", "v9", "v8"}
	branchMajorVersion := "v10"

	if len(latestMajorVersions) == 0 {
		return []pipeline{}
	}

	triggers := []*TriggerInfo{
		NewPromoteTrigger(branchMajorVersion),
		NewCronTrigger(latestMajorVersions),
	}

	if ConfigureForPRTestingOnly {
		triggers = append(triggers, NewTestTrigger(PRBranch, branchMajorVersion))
	}

	pipelines := make([]pipeline, 0, len(triggers))
	for _, trigger := range triggers {
		pipelines = append(pipelines, trigger.buildPipelines()...)
	}

	return pipelines
}

type TriggerInfo struct {
	Trigger           trigger
	Name              string
	SupportedVersions []*releaseVersion
	SetupSteps        []step
}

func NewTestTrigger(triggerBranch, testMajorVersion string) *TriggerInfo {
	baseTrigger := NewCronTrigger([]string{testMajorVersion})
	baseTrigger.Name = "Test trigger on push"
	baseTrigger.Trigger = trigger{
		Repo:   triggerRef{Include: []string{"gravitational/teleport"}},
		Event:  triggerRef{Include: []string{"push"}},
		Branch: triggerRef{Include: []string{triggerBranch}},
	}

	return baseTrigger
}

func NewPromoteTrigger(branchMajorVersion string) *TriggerInfo {
	promoteTrigger := triggerPromote
	promoteTrigger.Target.Include = append(promoteTrigger.Target.Include, "promote-docker")

	return &TriggerInfo{
		Trigger: promoteTrigger,
		Name:    "promote",
		SupportedVersions: []*releaseVersion{
			{
				MajorVersion:        branchMajorVersion,
				ShellVersion:        "$DRONE_TAG",
				RelativeVersionName: "drone-tag",
			},
		},
		SetupSteps: verifyValidPromoteRunSteps(),
	}
}

func NewCronTrigger(latestMajorVersions []string) *TriggerInfo {
	if len(latestMajorVersions) == 0 {
		return nil
	}

	majorVersionVarDirectory := "/go/vars/full-version"

	supportedVersions := make([]*releaseVersion, 0, len(latestMajorVersions))
	if len(latestMajorVersions) > 0 {
		latestMajorVersion := latestMajorVersions[0]
		supportedVersions = append(supportedVersions, &releaseVersion{
			MajorVersion:        latestMajorVersion,
			ShellVersion:        readCronShellVersionCommand(majorVersionVarDirectory, latestMajorVersion),
			RelativeVersionName: "current-version",
			SetupSteps:          []step{getLatestSemverStep(latestMajorVersion, majorVersionVarDirectory)},
		})

		if len(latestMajorVersions) > 1 {
			for i, majorVersion := range latestMajorVersions[1:] {
				supportedVersions = append(supportedVersions, &releaseVersion{
					MajorVersion:        majorVersion,
					ShellVersion:        readCronShellVersionCommand(majorVersionVarDirectory, majorVersion),
					RelativeVersionName: fmt.Sprintf("previous-version-%d", i+1),
					SetupSteps:          []step{getLatestSemverStep(majorVersion, majorVersionVarDirectory)},
				})
			}
		}
	}

	return &TriggerInfo{
		Trigger:           cronTrigger([]string{"teleport-container-images-cron"}),
		Name:              "cron",
		SupportedVersions: supportedVersions,
	}
}

func getLatestSemverStep(majorVersion string, majorVersionVarDirectory string) step {
	// We don't use "/go/src/github.com/gravitational/teleport" here as a later stage
	// may need to clone a different version, and "/go" persists between steps
	cloneDirectory := "/tmp/teleport"
	majorVersionVarPath := path.Join(majorVersionVarDirectory, majorVersion)
	return step{
		Name:  fmt.Sprintf("Find the latest available semver for %s", majorVersion),
		Image: "golang:1.18",
		Commands: append(
			cloneRepoCommands(cloneDirectory, fmt.Sprintf("branch/%s", majorVersion)),
			fmt.Sprintf("mkdir -pv %q", majorVersionVarDirectory),
			fmt.Sprintf("cd %q", path.Join(cloneDirectory, "build.assets", "tooling", "cmd", "query-latest")),
			fmt.Sprintf("go run . %q > %q", majorVersion, majorVersionVarPath),
			fmt.Sprintf("echo Found full semver \"$(cat %q)\" for major version %q", majorVersionVarPath, majorVersion),
		),
	}
}

func readCronShellVersionCommand(majorVersionDirectory, majorVersion string) string {
	return fmt.Sprintf("$(cat '%s')", path.Join(majorVersionDirectory, majorVersion))
}

// Drone triggers must all evaluate to "true" for a pipeline to be executed.
// As a result these pipelines are duplicated for each trigger.
// See https://docs.drone.io/pipeline/triggers/ for details.
func (ti *TriggerInfo) buildPipelines() []pipeline {
	pipelines := make([]pipeline, 0, len(ti.SupportedVersions))
	for _, teleportVersion := range ti.SupportedVersions {
		pipeline := teleportVersion.buildVersionPipeline(ti.SetupSteps)
		pipeline.Name += "-" + ti.Name
		pipeline.Trigger = ti.Trigger

		pipelines = append(pipelines, pipeline)
	}

	return pipelines
}

type releaseVersion struct {
	MajorVersion        string // This is the major version of a given build. `SearchVersion` should match this when evaluated.
	ShellVersion        string // This value will be evaluated by the shell in the context of a Drone step
	RelativeVersionName string // The set of values for this should not change between major releases
	SetupSteps          []step // Version-specific steps that must be ran before executing build and push steps
}

func (rv *releaseVersion) buildVersionPipeline(triggerSetupSteps []step) pipeline {
	pipelineName := fmt.Sprintf("teleport-container-images-%s", rv.RelativeVersionName)

	setupSteps, dependentStepNames := rv.getSetupStepInformation(triggerSetupSteps)

	pipeline := newKubePipeline(pipelineName)
	pipeline.Workspace = workspace{Path: "/go"}
	pipeline.Services = []service{
		dockerService(),
		dockerRegistryService(),
	}
	pipeline.Volumes = dockerVolumes()
	pipeline.Environment = map[string]value{
		"DEBIAN_FRONTEND": {
			raw: "noninteractive",
		},
	}
	pipeline.Steps = append(setupSteps, rv.buildSteps(dependentStepNames)...)

	return pipeline
}

func (rv *releaseVersion) getSetupStepInformation(triggerSetupSteps []step) ([]step, []string) {
	triggerSetupStepNames := make([]string, 0, len(triggerSetupSteps))
	for _, triggerSetupStep := range triggerSetupSteps {
		triggerSetupStepNames = append(triggerSetupStepNames, triggerSetupStep.Name)
	}

	nextStageSetupStepNames := triggerSetupStepNames
	if len(rv.SetupSteps) > 0 {
		versionSetupStepNames := make([]string, 0, len(rv.SetupSteps))
		for _, versionSetupStep := range rv.SetupSteps {
			versionSetupStep.DependsOn = append(versionSetupStep.DependsOn, triggerSetupStepNames...)
			versionSetupStepNames = append(versionSetupStepNames, versionSetupStep.Name)
		}

		nextStageSetupStepNames = versionSetupStepNames
	}

	setupSteps := append(triggerSetupSteps, rv.SetupSteps...)

	return setupSteps, nextStageSetupStepNames
}

func (rv *releaseVersion) buildSteps(setupStepNames []string) []step {
	clonedRepoPath := "/go/src/github.com/gravitational/teleport"
	steps := make([]step, 0)

	setupSteps := []step{
		waitForDockerStep(),
		waitForDockerRegistryStep(),
		cloneRepoStep(clonedRepoPath, rv.ShellVersion),
	}
	for _, setupStep := range setupSteps {
		setupStep.DependsOn = append(setupStep.DependsOn, setupStepNames...)
		steps = append(steps, setupStep)
		setupStepNames = append(setupStepNames, setupStep.Name)
	}

	for _, product := range rv.getProducts(clonedRepoPath) {
		steps = append(steps, product.buildSteps(rv, setupStepNames)...)
	}

	return steps
}

func (rv *releaseVersion) getProducts(clonedRepoPath string) []*Product {
	ossTeleport := NewTeleportProduct(false, false, rv)
	teleportProducts := []*Product{
		ossTeleport,                         // OSS
		NewTeleportProduct(true, false, rv), // Enterprise
		NewTeleportProduct(true, true, rv),  // Enterprise/FIPS
	}
	teleportLabProducts := []*Product{
		// If we want to add Enterprise and/or Enterprise/FIPS Teleport Lab support in the future
		// it should be configured here
		NewTeleportLabProduct(clonedRepoPath, rv, ossTeleport),
	}
	teleportOperatorProduct := NewTeleportOperatorProduct(clonedRepoPath)

	products := make([]*Product, 0, len(teleportProducts)+len(teleportLabProducts)+1)
	products = append(products, teleportProducts...)
	products = append(products, teleportLabProducts...)
	products = append(products, teleportOperatorProduct)

	return products
}

func (rv *releaseVersion) getTagsForVersion() []*imageTag {
	return []*imageTag{
		{
			ShellBaseValue:   fmt.Sprintf("$(echo %s | sed 's/v//' | cut -d'.' -f 1)", rv.ShellVersion),
			DisplayBaseValue: "major",
		},
		{
			ShellBaseValue:   fmt.Sprintf("$(echo %s | sed 's/v//' | cut -d'.' -f 1,2)", rv.ShellVersion),
			DisplayBaseValue: "minor",
		},
		{
			ShellBaseValue:   fmt.Sprintf("$(echo %s | sed 's/v//' | cut -d'.' -f 1,2,3)", rv.ShellVersion),
			DisplayBaseValue: "canonical",
		},
	}
}

type image struct {
	Repo string
	Name string
	Tag  *imageTag
}

func (i *image) GetShellName() string {
	repo := ""
	if !i.IsLocalImage() {
		// Ensure one and only one "/"
		repo = strings.TrimSuffix(i.Repo, "/")
		repo += "/"
	}

	return fmt.Sprintf("%s%s:%s", repo, i.Name, i.Tag.GetShellValue())
}

func (i *image) GetDisplayName() string {
	return fmt.Sprintf("%s:%s", i.Name, i.Tag.GetDisplayValue())
}

func (i *image) IsLocalImage() bool {
	return i.Repo == ""
}

type Product struct {
	Name                 string
	DockerfilePath       string
	WorkingDirectory     string
	DockerfileTarget     string
	SupportedArchs       []string
	SetupSteps           []step
	DockerfileArgBuilder func(arch string) []string
	ImageBuilder         func(repo string, tag *imageTag) *image
	GetRequiredStepNames func(arch string) []string
}

func NewTeleportProduct(isEnterprise, isFips bool, version *releaseVersion) *Product {
	workingDirectory := "/go/build"
	downloadURL := "https://raw.githubusercontent.com/gravitational/teleport/${DRONE_SOURCE_BRANCH:-master}/build.assets/charts/Dockerfile"
	name := "teleport"
	dockerfileTarget := "teleport"
	supportedArches := []string{"amd64"}

	if isEnterprise {
		name += "-ent"
	}
	if isFips {
		dockerfileTarget += "-fips"
		name += "-fips"
	} else {
		supportedArches = append(supportedArches, "arm", "arm64")
	}

	setupStep, debPaths, dockerfilePath := teleportSetupStep(version.ShellVersion, name, workingDirectory, downloadURL, supportedArches)

	return &Product{
		Name:             name,
		DockerfilePath:   dockerfilePath,
		WorkingDirectory: workingDirectory,
		DockerfileTarget: dockerfileTarget,
		SupportedArchs:   supportedArches,
		SetupSteps:       []step{setupStep},
		DockerfileArgBuilder: func(arch string) []string {
			return []string{
				fmt.Sprintf("DEB_PATH=%s", debPaths[arch]),
			}
		},
		ImageBuilder: func(repo string, tag *imageTag) *image {
			imageProductName := "teleport"
			if isEnterprise {
				imageProductName += "-ent"
			}

			if isFips {
				tag.AppendString("fips")
			}

			return &image{
				Repo: repo,
				Name: imageProductName,
				Tag:  tag,
			}
		},
	}
}

func NewTeleportLabProduct(cloneDirectory string, version *releaseVersion, teleport *Product) *Product {
	workingDirectory := path.Join(cloneDirectory, "docker", "sshd")
	dockerfile := path.Join(cloneDirectory, "docker", "sshd", "Dockerfile")
	name := "teleport-lab"

	return &Product{
		Name:             name,
		DockerfilePath:   dockerfile,
		WorkingDirectory: workingDirectory,
		SupportedArchs:   teleport.SupportedArchs,
		DockerfileArgBuilder: func(arch string) []string {
			return []string{
				fmt.Sprintf("BASE_IMAGE=%s", teleport.GetLocalRegistryImage(arch, version).GetShellName()),
			}
		},
		ImageBuilder: func(repo string, tag *imageTag) *image {
			return &image{
				Repo: repo,
				Name: name,
				Tag:  tag,
			}
		},
		GetRequiredStepNames: func(arch string) []string {
			return []string{teleport.GetBuildStepName(arch, version)}
		},
	}
}

func NewTeleportOperatorProduct(cloneDirectory string) *Product {
	name := "teleport-operator"
	return &Product{
		Name:             name,
		DockerfilePath:   path.Join(cloneDirectory, "operator", "Dockerfile"),
		WorkingDirectory: cloneDirectory,
		SupportedArchs:   []string{"amd64", "arm", "arm64"},
		ImageBuilder: func(repo string, tag *imageTag) *image {
			return &image{
				Repo: repo,
				Name: name,
				Tag:  tag,
			}
		},
		DockerfileArgBuilder: func(arch string) []string {
			gccPackage := ""
			compilerName := ""
			switch arch {
			case "x86_64":
				fallthrough
			case "amd64":
				gccPackage = "gcc-x86-64-linux-gnu"
				compilerName = "x86_64-linux-gnu-gcc"
			case "i686":
				fallthrough
			case "i386":
				gccPackage = "gcc-multilib-i686-linux-gnu"
				compilerName = "i686-linux-gnu-gcc"
			case "aarch64":
				fallthrough
			case "arm64":
				gccPackage = "gcc-aarch64-linux-gnu"
				compilerName = "aarch64-linux-gnu-gcc"
			// We may want to add additional arm ISAs in the future to support devices without hardware FPUs
			case "armhf":
			case "arm":
				gccPackage = "gcc-arm-linux-gnueabihf"
				compilerName = "arm-linux-gnueabihf-gcc"
			}

			return []string{
				fmt.Sprintf("COMPILER_PACKAGE=%s", gccPackage),
				fmt.Sprintf("COMPILER_NAME=%s", compilerName),
			}
		},
	}
}

func teleportSetupStep(shellVersion, packageName, workingPath, downloadURL string, archs []string) (step, map[string]string, string) {
	keyPath := "/usr/share/keyrings/teleport-archive-keyring.asc"
	downloadDirectory := "/tmp/apt-download"
	timeout := 30 * 60 // 30 minutes in seconds
	sleepTime := 15    // 15 seconds
	dockerfilePath := path.Join(workingPath, "Dockerfile")

	commands := []string{
		// Setup the environment
		fmt.Sprintf("PACKAGE_NAME=%q", packageName),
		fmt.Sprintf("PACKAGE_VERSION=%q", shellVersion),
		"apt update",
		"apt install --no-install-recommends -y ca-certificates curl",
		"update-ca-certificates",
		// Download the dockerfile
		fmt.Sprintf("mkdir -pv $(dirname %q)", dockerfilePath),
		fmt.Sprintf("curl -Ls -o %q %q", dockerfilePath, downloadURL),
		// Add the Teleport APT repo
		fmt.Sprintf("curl https://apt.releases.teleport.dev/gpg -o %q", keyPath),
		". /etc/os-release",
		// Per https://docs.drone.io/pipeline/environment/syntax/#common-problems I'm using '$$' here to ensure
		// That the shell variable is not expanded until runtime, preventing drone from erroring on the
		// drone-unsupported '?'
		"MAJOR_VERSION=$(echo $${PACKAGE_VERSION?} | cut -d'.' -f 1)",
		fmt.Sprintf("echo \"deb [signed-by=%s] https://apt.releases.teleport.dev/$${ID?} $${VERSION_CODENAME?} stable/$${MAJOR_VERSION?}\""+
			" > /etc/apt/sources.list.d/teleport.list", keyPath),
		fmt.Sprintf("END_TIME=$(( $(date +%%s) + %d ))", timeout),
		"TRIMMED_VERSION=$(echo $${PACKAGE_VERSION} | cut -d'v' -f 2)",
		"TIMED_OUT=true",
		// Poll APT until the timeout is reached or the package becomes available
		"while [ $(date +%s) -lt $${END_TIME?} ]; do",
		"echo 'Running apt update...'",
		// This will error on new major versions where the "stable/$${MAJOR_VERSION}" component doesn't exist yet, so we ignore it here.
		"apt update > /dev/null || true",
		"[ $(apt-cache madison $${PACKAGE_NAME} | grep $${TRIMMED_VERSION?} | wc -l) -ge 1 ] && TIMED_OUT=false && break;",
		fmt.Sprintf("echo 'Package not found yet, waiting another %d seconds...'", sleepTime),
		fmt.Sprintf("sleep %d", sleepTime),
		"done",
		// Log success or failure and record full version string
		"[ $${TIMED_OUT?} = true ] && echo \"Timed out while looking for APT package \\\"$${PACKAGE_NAME}\\\" matching \\\"$${TRIMMED_VERSION}\\\"\" && exit 1",
		"FULL_VERSION=$(apt-cache madison $${PACKAGE_NAME} | grep $${TRIMMED_VERSION} | cut -d'|' -f 2 | tr -d ' ' | head -n 1)",
		fmt.Sprintf("echo \"Found APT package, downloading \\\"$${PACKAGE_NAME}=$${FULL_VERSION}\\\" for %q...\"", strings.Join(archs, "\", \"")),
		fmt.Sprintf("mkdir -pv %q", downloadDirectory),
		fmt.Sprintf("cd %q", downloadDirectory),
	}

	for _, arch := range archs {
		// Our built debs are listed as ISA "armhf" not "arm", so we account for that here
		if arch == "arm" {
			arch = "armhf"
		}

		commands = append(commands, []string{
			// This will allow APT to download other architectures
			fmt.Sprintf("dpkg --add-architecture %q", arch),
		}...)
	}

	// This will error due to Ubuntu's APT repo structure but it doesn't matter here
	commands = append(commands, "apt update &> /dev/null || true")

	archDestFileMap := make(map[string]string, len(archs))
	for _, arch := range archs {
		relArchDir := path.Join(".", "/artifacts/deb/", packageName, arch)
		archDir := path.Join(workingPath, relArchDir)
		// Example: `./artifacts/deb/teleport-ent/arm64/v10.1.4.deb`
		relDestPath := path.Join(relArchDir, fmt.Sprintf("%s.deb", shellVersion))
		// Example: `/go/./artifacts/deb/teleport-ent/arm64/v10.1.4.deb`
		destPath := path.Join(workingPath, relDestPath)

		archDestFileMap[arch] = relDestPath

		// Our built debs are listed as ISA "armhf" not "arm", so we account for that here
		if arch == "arm" {
			arch = "armhf"
		}

		// This could probably be parallelized to slightly reduce runtime
		fullPackageName := fmt.Sprintf("%s:%s=$${FULL_VERSION}", packageName, arch)
		commands = append(commands, []string{
			fmt.Sprintf("mkdir -pv %q", archDir),
			fmt.Sprintf("apt download %q", fullPackageName),
			"FILENAME=$(ls)", // This will only return the download file as it is the only file in that directory
			"echo \"Downloaded file \\\"$${FILENAME}\\\"\"",
			fmt.Sprintf("mv \"$${FILENAME}\" %q", path.Join(archDir, "$${PACKAGE_VERSION}.deb")),
			fmt.Sprintf("echo Downloaded %q to %q", fullPackageName, destPath),
		}...)
	}

	return step{
		Name:     fmt.Sprintf("Download %q Dockerfile and DEB artifacts from APT", packageName),
		Image:    "ubuntu:22.04",
		Commands: commands,
	}, archDestFileMap, dockerfilePath
}

func (p *Product) GetLocalRegistryImage(arch string, version *releaseVersion) *image {
	return &image{
		Repo: LocalRegistry,
		Name: p.Name,
		Tag: &imageTag{
			ShellBaseValue:   version.ShellVersion,
			DisplayBaseValue: version.MajorVersion,
			Arch:             arch,
		},
	}
}

func (p *Product) buildSteps(version *releaseVersion, setupStepNames []string) []step {
	containerRepos := GetContainerRepos()

	steps := make([]step, 0)

	for _, setupStep := range p.SetupSteps {
		setupStep.DependsOn = append(setupStep.DependsOn, setupStepNames...)
		steps = append(steps, setupStep)
		setupStepNames = append(setupStepNames, setupStep.Name)
	}

	archBuildStepDetails := make([]*buildStepOutput, 0, len(p.SupportedArchs))
	for _, supportedArch := range p.SupportedArchs {
		archBuildStep, archBuildStepDetail := p.createBuildStep(supportedArch, version)

		archBuildStep.DependsOn = append(archBuildStep.DependsOn, setupStepNames...)
		if p.GetRequiredStepNames != nil {
			archBuildStep.DependsOn = append(archBuildStep.DependsOn, p.GetRequiredStepNames(supportedArch)...)
		}

		steps = append(steps, archBuildStep)
		archBuildStepDetails = append(archBuildStepDetails, archBuildStepDetail)
	}

	for _, containerRepo := range containerRepos {
		steps = append(steps, containerRepo.buildSteps(archBuildStepDetails)...)
	}

	return steps
}

func (p *Product) GetBuildStepName(arch string, version *releaseVersion) string {
	telportImageName := p.GetLocalRegistryImage(arch, version)
	return fmt.Sprintf("Build %s image %q", p.Name, telportImageName.GetDisplayName())
}

func (p *Product) createBuildStep(arch string, version *releaseVersion) (step, *buildStepOutput) {
	localRegistryImage := p.GetLocalRegistryImage(arch, version)
	builderName := fmt.Sprintf("%s-builder", localRegistryImage.GetDisplayName())

	buildxConfigFileDir := path.Join("/tmp", builderName)
	buildxConfigFilePath := path.Join(buildxConfigFileDir, "buildkitd.toml")

	buildxCreateCommand := "docker buildx create"
	buildxCreateCommand += fmt.Sprintf(" --driver %q", "docker-container")
	// This is set so that buildx can reach the local registry
	buildxCreateCommand += fmt.Sprintf(" --driver-opt %q", "network=host")
	buildxCreateCommand += fmt.Sprintf(" --name %q", builderName)
	buildxCreateCommand += fmt.Sprintf(" --config %q", buildxConfigFilePath)

	buildCommand := "docker buildx build"
	buildCommand += " --push"
	buildCommand += fmt.Sprintf(" --builder %q", builderName)
	if p.DockerfileTarget != "" {
		buildCommand += fmt.Sprintf(" --target %q", p.DockerfileTarget)
	}
	buildCommand += fmt.Sprintf(" --platform %q", "linux/"+arch)
	buildCommand += fmt.Sprintf(" --tag %q", localRegistryImage.GetShellName())
	buildCommand += fmt.Sprintf(" --file %q", p.DockerfilePath)
	if p.DockerfileArgBuilder != nil {
		for _, buildArg := range p.DockerfileArgBuilder(arch) {
			buildCommand += fmt.Sprintf(" --build-arg %q", buildArg)
		}
	}
	buildCommand += " " + p.WorkingDirectory

	step := step{
		Name:    p.GetBuildStepName(arch, version),
		Image:   "docker",
		Volumes: dockerVolumeRefs(),
		Environment: map[string]value{
			"DOCKER_BUILDKIT": {
				raw: "1",
			},
		},
		Commands: []string{
			"docker run --privileged --rm tonistiigi/binfmt --install all",
			fmt.Sprintf("mkdir -pv %q && cd %q", p.WorkingDirectory, p.WorkingDirectory),
			fmt.Sprintf("mkdir -pv %q", buildxConfigFileDir),
			fmt.Sprintf("echo '[registry.%q]' > %q", LocalRegistry, buildxConfigFilePath),
			fmt.Sprintf("echo '  http = true' >> %q", buildxConfigFilePath),
			buildxCreateCommand,
			buildCommand,
			fmt.Sprintf("docker buildx rm %q", builderName),
			fmt.Sprintf("rm -rf %q", buildxConfigFileDir),
		},
	}

	return step, &buildStepOutput{
		StepName:   step.Name,
		BuiltImage: localRegistryImage,
		Version:    version,
		Product:    p,
	}
}

// The `step` struct doesn't contain enough information to setup
// dependent steps so we add that via this struct
type buildStepOutput struct {
	StepName   string
	BuiltImage *image
	Version    *releaseVersion
	Product    *Product
}

type ContainerRepo struct {
	Name           string
	Environment    map[string]value
	RegistryDomain string
	RegistryOrg    string
	LoginCommands  []string
	TagBuilder     func(baseTag *imageTag) *imageTag // Postprocessor for tags that append CR-specific suffixes
}

func NewEcrContainerRepo(accessKeyIDSecret, secretAccessKeySecret, domain string, isStaging bool) *ContainerRepo {
	nameSuffix := "production"
	ecrRegion := PublicEcrRegion
	loginSubcommand := "ecr-public"
	if isStaging {
		nameSuffix = "staging"
		ecrRegion = StagingEcrRegion
		loginSubcommand = "ecr"
	}

	registryOrg := ProductionRegistryOrg
	if ConfigureForPRTestingOnly {
		accessKeyIDSecret = TestingSecretPrefix + accessKeyIDSecret
		secretAccessKeySecret = TestingSecretPrefix + secretAccessKeySecret
		registryOrg = TestingECRRegistryOrg

		if isStaging {
			domain = ECRTestingDomain
			ecrRegion = TestingEcrRegion
		}
	}

	return &ContainerRepo{
		Name: fmt.Sprintf("ECR - %s", nameSuffix),
		Environment: map[string]value{
			"AWS_ACCESS_KEY_ID": {
				fromSecret: accessKeyIDSecret,
			},
			"AWS_SECRET_ACCESS_KEY": {
				fromSecret: secretAccessKeySecret,
			},
		},
		RegistryDomain: domain,
		RegistryOrg:    registryOrg,
		LoginCommands: []string{
			"apk add --no-cache aws-cli",
			"TIMESTAMP=$(date -d @\"$DRONE_BUILD_CREATED\" '+%Y%m%d%H%M')",
			fmt.Sprintf("aws %s get-login-password --region=%s | docker login -u=\"AWS\" --password-stdin %s", loginSubcommand, ecrRegion, domain),
		},
		TagBuilder: func(tag *imageTag) *imageTag {
			if isStaging {
				tag.AppendString("$TIMESTAMP")
			}

			return tag
		},
	}
}

func NewQuayContainerRepo(dockerUsername, dockerPassword string) *ContainerRepo {
	registryOrg := ProductionRegistryOrg
	if ConfigureForPRTestingOnly {
		dockerUsername = TestingSecretPrefix + dockerUsername
		dockerPassword = TestingSecretPrefix + dockerPassword
		registryOrg = TestingQuayRegistryOrg
	}

	return &ContainerRepo{
		Name: "Quay",
		Environment: map[string]value{
			"QUAY_USERNAME": {
				fromSecret: dockerUsername,
			},
			"QUAY_PASSWORD": {
				fromSecret: dockerPassword,
			},
		},
		RegistryDomain: ProductionRegistryQuay,
		RegistryOrg:    registryOrg,
		LoginCommands: []string{
			fmt.Sprintf("docker login -u=\"$QUAY_USERNAME\" -p=\"$QUAY_PASSWORD\" %q", ProductionRegistryQuay),
		},
	}
}

func GetContainerRepos() []*ContainerRepo {
	return []*ContainerRepo{
		NewQuayContainerRepo("PRODUCTION_QUAYIO_DOCKER_USERNAME", "PRODUCTION_QUAYIO_DOCKER_PASSWORD"),
		NewEcrContainerRepo("STAGING_TELEPORT_DRONE_USER_ECR_KEY", "STAGING_TELEPORT_DRONE_USER_ECR_SECRET", StagingRegistry, true),
		NewEcrContainerRepo("PRODUCTION_TELEPORT_DRONE_USER_ECR_KEY", "PRODUCTION_TELEPORT_DRONE_USER_ECR_SECRET", ProductionRegistry, false),
	}
}

func (cr *ContainerRepo) buildSteps(buildStepDetails []*buildStepOutput) []step {
	if len(buildStepDetails) == 0 {
		return nil
	}

	steps := make([]step, 0)

	imageTags := cr.BuildImageTags(buildStepDetails[0].Version)
	pushedImages := make(map[*imageTag][]*image, len(imageTags))
	pushStepNames := make([]string, 0, len(buildStepDetails))
	for _, buildStepDetail := range buildStepDetails {
		pushStep, pushedArchImages := cr.tagAndPushStep(buildStepDetail, imageTags)
		pushStepNames = append(pushStepNames, pushStep.Name)
		for _, imageTag := range imageTags {
			pushedImages[imageTag] = append(pushedImages[imageTag], pushedArchImages[imageTag])
		}

		steps = append(steps, pushStep)
	}

	imageRepo := cr.BuildImageRepo()
	for _, imageTag := range imageTags {
		multiarchImageTag := *imageTag
		multiarchImageTag.Arch = ""
		manifestImage := buildStepDetails[0].Product.ImageBuilder(imageRepo, &multiarchImageTag)
		manifestStepName := cr.createAndPushManifestStep(manifestImage, pushStepNames, pushedImages[imageTag])
		steps = append(steps, manifestStepName)
	}

	return steps
}

func (cr *ContainerRepo) logoutCommand() string {
	return fmt.Sprintf("docker logout %q", cr.RegistryDomain)
}

func (cr *ContainerRepo) buildCommandsWithLogin(wrappedCommands []string) []string {
	commands := make([]string, 0)
	commands = append(commands, cr.LoginCommands...)
	commands = append(commands, wrappedCommands...)
	commands = append(commands, cr.logoutCommand())

	return commands
}

func (cr *ContainerRepo) BuildImageRepo() string {
	return fmt.Sprintf("%s/%s/", cr.RegistryDomain, cr.RegistryOrg)
}

type imageTag struct {
	ShellBaseValue   string // Should evaluate in a shell context to the tag's value
	DisplayBaseValue string // Should be set to a human-readable version of ShellTag
	Arch             string
}

func NewLatestTag() *imageTag {
	return &imageTag{
		ShellBaseValue:   "latest",
		DisplayBaseValue: "latest",
	}
}

func (it *imageTag) AppendString(s string) {
	it.ShellBaseValue += fmt.Sprintf("-%s", s)
	it.DisplayBaseValue += fmt.Sprintf("-%s", s)
}

func (it *imageTag) IsMultArch() bool {
	return it.Arch != ""
}

func (it *imageTag) GetShellValue() string {
	return it.getValue(it.ShellBaseValue)
}

func (it *imageTag) GetDisplayValue() string {
	return it.getValue(it.DisplayBaseValue)
}

func (it *imageTag) getValue(baseValue string) string {
	if it.Arch == "" {
		return baseValue
	}

	return fmt.Sprintf("%s-%s", baseValue, it.Arch)
}

func (cr *ContainerRepo) BuildImageTags(version *releaseVersion) []*imageTag {
	tags := version.getTagsForVersion()

	if cr.TagBuilder != nil {
		for i, tag := range tags {
			tags[i] = cr.TagBuilder(tag)
		}
	}

	return tags
}

func (cr *ContainerRepo) tagAndPushStep(buildStepDetails *buildStepOutput, imageTags []*imageTag) (step, map[*imageTag]*image) {
	imageRepo := cr.BuildImageRepo()

	archImages := make(map[*imageTag]*image, len(imageTags))
	for _, imageTag := range imageTags {
		archTag := *imageTag
		archTag.Arch = buildStepDetails.BuiltImage.Tag.Arch
		archImages[imageTag] = buildStepDetails.Product.ImageBuilder(imageRepo, &archTag)
	}

	commands := []string{
		fmt.Sprintf("docker pull %q", buildStepDetails.BuiltImage.GetShellName()), // This will pull from the local registry
	}
	for _, archImage := range archImages {
		commands = append(commands, fmt.Sprintf("docker tag %q %q", buildStepDetails.BuiltImage.GetShellName(), archImage.GetShellName()))
	}
	for _, archImage := range archImages {
		commands = append(commands, fmt.Sprintf("docker push %q", archImage.GetShellName()))
	}

	step := step{
		Name:        fmt.Sprintf("Tag and push image %q to %s", buildStepDetails.BuiltImage.GetDisplayName(), cr.Name),
		Image:       "docker",
		Volumes:     dockerVolumeRefs(),
		Environment: cr.Environment,
		Commands:    cr.buildCommandsWithLogin(commands),
		DependsOn: []string{
			buildStepDetails.StepName,
		},
	}

	return step, archImages
}

func (cr *ContainerRepo) createAndPushManifestStep(manifestImage *image, pushStepNames []string, pushedImages []*image) step {
	if len(pushStepNames) == 0 {
		return step{}
	}

	manifestCommandArgs := make([]string, 0, len(pushedImages))
	for _, pushedImage := range pushedImages {
		manifestCommandArgs = append(manifestCommandArgs, fmt.Sprintf("--amend %q", pushedImage.GetShellName()))
	}

	return step{
		Name:        fmt.Sprintf("Create manifest and push %q to %s", manifestImage.GetDisplayName(), cr.Name),
		Image:       "docker",
		Volumes:     dockerVolumeRefs(),
		Environment: cr.Environment,
		Commands: cr.buildCommandsWithLogin([]string{
			fmt.Sprintf("docker manifest create %q %s", manifestImage.GetShellName(), strings.Join(manifestCommandArgs, " ")),
			fmt.Sprintf("docker manifest push %q", manifestImage.GetShellName()),
		}),
		DependsOn: pushStepNames,
	}
}
