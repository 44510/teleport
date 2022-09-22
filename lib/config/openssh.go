/*

 Copyright 2022 Gravitational, Inc.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.


*/

package config

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/coreos/go-semver/semver"
	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/tbot/identity"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

var (
	// openSSHVersionRegex is a regex used to parse OpenSSH version strings.
	openSSHVersionRegex = regexp.MustCompile(`^OpenSSH_(?P<major>\d+)\.(?P<minor>\d+)(?:p(?P<patch>\d+))?`)

	// openSSHMinVersionForRSAWorkaround is the OpenSSH version after which the
	// RSA deprecation workaround should be added to generated ssh_config.
	openSSHMinVersionForRSAWorkaround = semver.New("8.5.0")

	// openSSHMinVersionForHostAlgos is the first version that understands all host keys required by us.
	// HostKeyAlgorithms will be added to ssh config if the version is above listed here.
	openSSHMinVersionForHostAlgos = semver.New("7.8.0")
)

type SSHConfigGenerator struct {
	getSSHVersion     func() (*semver.Version, error)
	getExecutablePath func() (string, error)
}

func NewCustomSSHConfigGenerator(
	getSSHVersion func() (*semver.Version, error),
	getExecutablePath func() (string, error),
) *SSHConfigGenerator {
	return &SSHConfigGenerator{
		getSSHVersion:     getSSHVersion,
		getExecutablePath: getExecutablePath,
	}
}

// parseSSHVersion attempts to parse the local SSH version, used to determine
// certain config template parameters for client version compatibility.
func parseSSHVersion(versionString string) (*semver.Version, error) {
	versionTokens := strings.Split(versionString, " ")
	if len(versionTokens) == 0 {
		return nil, trace.BadParameter("invalid version string: %s", versionString)
	}

	versionID := versionTokens[0]
	matches := openSSHVersionRegex.FindStringSubmatch(versionID)
	if matches == nil {
		return nil, trace.BadParameter("cannot parse version string: %q", versionID)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, trace.Wrap(err, "invalid major version number: %s", matches[1])
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, trace.Wrap(err, "invalid minor version number: %s", matches[2])
	}

	patch := 0
	if matches[3] != "" {
		patch, err = strconv.Atoi(matches[3])
		if err != nil {
			return nil, trace.Wrap(err, "invalid patch version number: %s", matches[3])
		}
	}

	return &semver.Version{
		Major: int64(major),
		Minor: int64(minor),
		Patch: int64(patch),
	}, nil
}

// getSystemSSHVersion attempts to query the system SSH for its current version.
func getSystemSSHVersion() (*semver.Version, error) {
	var out bytes.Buffer

	cmd := exec.Command("ssh", "-V")
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return parseSSHVersion(out.String())
}

func (s *SSHConfigGenerator) CheckAndSetDefaults() {
	if s.getSSHVersion == nil {
		s.getSSHVersion = getSystemSSHVersion
	}
	if s.getExecutablePath == nil {
		s.getExecutablePath = os.Executable
	}
}

func (s *SSHConfigGenerator) GenerateSSHConfig(params *SSHConfigParameters) (strings.Builder, error) {
	s.CheckAndSetDefaults()

	if err := params.CheckAndSetDefaults(); err != nil {
		return strings.Builder{}, trace.Wrap(err)
	}

	// Default to including the RSA deprecation workaround.
	rsaWorkaround := true
	version, err := s.getSSHVersion()
	if err != nil {
		log.WithError(err).Debugf("Could not determine SSH version, will include RSA workaround.")
	} else if version.LessThan(*openSSHMinVersionForRSAWorkaround) {
		log.Debugf("OpenSSH version %s does not require workaround for RSA deprecation", version)
		rsaWorkaround = false
	} else {
		log.Debugf("OpenSSH version %s will use workaround for RSA deprecation", version)
	}

	addHostAlgos := true
	if version.LessThan(*openSSHMinVersionForHostAlgos) {
		addHostAlgos = false
		log.Debugf("")
	} else {
		log.Debugf("OpenSSH version %s will add HostKeyAlgorithms to ssh config", version)
	}

	executablePath := params.ExecutablePath
	if executablePath == "" {
		executablePath, err = s.getExecutablePath()
		if err != nil {
			return strings.Builder{}, trace.Wrap(err)
		}
	}

	knownHostsPath := filepath.Join(params.DestinationDir, teleport.KnownHosts)
	identityFilePath := filepath.Join(params.DestinationDir, identity.PrivateKeyKey)
	certificateFilePath := filepath.Join(params.DestinationDir, identity.SSHCertKey)

	var sshConfigBuilder strings.Builder
	if err := sshConfigTemplate.Execute(&sshConfigBuilder, SSHConfigParameters{
		AppName:                  params.AppName,
		ClusterName:              params.ClusterName,
		ProxyHost:                params.ProxyHost,
		KnownHostsPath:           knownHostsPath,
		IdentityFilePath:         identityFilePath,
		CertificateFilePath:      certificateFilePath,
		IncludeRSAWorkaround:     rsaWorkaround,
		IncludeTeleportHostAlgos: addHostAlgos,
		ExecutablePath:           executablePath,
		DestinationDir:           params.DestinationDir,
	}); err != nil {
		return strings.Builder{}, trace.Wrap(err)
	}

	return sshConfigBuilder, nil
}

type SSHConfigParameters struct {
	AppName             string
	ClusterName         string
	KnownHostsPath      string
	IdentityFilePath    string
	CertificateFilePath string
	ProxyHost           string
	ExecutablePath      string
	DestinationDir      string

	// IncludeRSAWorkaround controls whether the RSA deprecation workaround is
	// included in the generated configuration. Newer versions of OpenSSH
	// deprecate RSA certificates and, due to a bug in golang's ssh package,
	// Teleport wrongly advertises its unaffected certificates as a
	// now-deprecated certificate type. The workaround includes a config
	// override to re-enable RSA certs for just Teleport hosts, however it is
	// only supported on OpenSSH 8.5 and later.
	IncludeRSAWorkaround bool

	IncludeTeleportHostAlgos bool
}

func (c *SSHConfigParameters) CheckAndSetDefaults() error {
	if c.AppName == "" {
		return trace.BadParameter("AppName is missing")
	}

	return nil
}

// TODO: remove PubkeyAcceptedKeyTypes once we finish deprecating SHA1
var sshConfigTemplate = template.Must(template.New("ssh-config").Parse(`
# Begin generated Teleport configuration for {{ .ProxyHost }} by {{ .AppName }}

# Common flags for all {{ .ClusterName }} hosts
Host *.{{ .ClusterName }} {{ .ProxyHost }}
    UserKnownHostsFile "{{ .KnownHostsPath }}"
    IdentityFile "{{ .IdentityFilePath }}"
    CertificateFile "{{ .CertificateFilePath }}"
    HostKeyAlgorithms ssh-rsa-cert-v01@openssh.com{{- if .IncludeRSAWorkaround }}
    PubkeyAcceptedAlgorithms +ssh-rsa-cert-v01@openssh.com{{- end }}{{- if .IncludeTeleportHostAlgos }}
    HostKeyAlgorithms rsa-sha2-256-cert-v01@openssh.com,rsa-sha2-512-cert-v01@openssh.com,ssh-rsa-cert-v01@openssh.com{{- end }}

# Flags for all {{ .ClusterName }} hosts except the proxy
Host *.{{ .ClusterName }} !{{ .ProxyHost }}
    Port 3022
{{- if eq .AppName "tsh" }}
    ProxyCommand "{{ .ExecutablePath }}" proxy ssh --cluster={{ .ClusterName }} --proxy={{ .ProxyHost }} %r@%h:%p
{{- end }}{{- if eq .AppName "tbot" }}
    ProxyCommand "{{ .ExecutablePath }}" proxy --destination-dir={{ .DestinationDir }} --proxy={{ .ProxyHost }} ssh --cluster={{ .ClusterName }}  %r@%h:%p
{{- end }}

# End generated Teleport configuration
`))
