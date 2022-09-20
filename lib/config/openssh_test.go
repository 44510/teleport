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
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/require"
)

func TestParseSSHVersion(t *testing.T) {
	tests := []struct {
		str     string
		version *semver.Version
		err     bool
	}{
		{
			str:     "OpenSSH_8.2p1 Ubuntu-4ubuntu0.4, OpenSSL 1.1.1f  31 Mar 2020",
			version: semver.New("8.2.1"),
		},
		{
			str:     "OpenSSH_8.8p1, OpenSSL 1.1.1m  14 Dec 2021",
			version: semver.New("8.8.1"),
		},
		{
			str:     "OpenSSH_7.5p1, OpenSSL 1.0.2s-freebsd  28 May 2019",
			version: semver.New("7.5.1"),
		},
		{
			str:     "OpenSSH_7.9p1 Raspbian-10+deb10u2, OpenSSL 1.1.1d  10 Sep 2019",
			version: semver.New("7.9.1"),
		},
		{
			// Couldn't find a full example but in theory patch is optional:
			str:     "OpenSSH_8.1 foo",
			version: semver.New("8.1.0"),
		},
		{
			str: "Teleport v8.0.0-dev.40 git:v8.0.0-dev.40-0-ge9194c256 go1.17.2",
			err: true,
		},
	}

	for _, test := range tests {
		version, err := parseSSHVersion(test.str)
		if test.err {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, version.Equal(*test.version), "got version = %v, want = %v", version, test.version)
		}
	}
}
