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

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInsecureWebClientHTTPProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "localhost:9999")
	client := NewInsecureWebClient()
	resp, err := client.Get("https://example.com")
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()
	// Client should try to proxy through nonexistent server at localhost.
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxyconnect")
	require.Contains(t, err.Error(), "connection refused")
}

func TestNewClientWithPoolHTTPProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "localhost:9999")
	client := newClientWithPool(nil)
	resp, err := client.Get("https://example.com")
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()
	// Client should try to proxy through nonexistent server at localhost.
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxyconnect")
	require.Contains(t, err.Error(), "connection refused")
}
