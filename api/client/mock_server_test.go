/*
Copyright 2023 Gravitational, Inc.

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
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"testing"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/utils/grpc/interceptors"
)

// mockServer mocks an Auth Server.
type mockServer struct {
	addr string
	grpc *grpc.Server
	*proto.UnimplementedAuthServiceServer
}

func newMockServer(addr string) *mockServer {
	m := &mockServer{
		addr: addr,
		grpc: grpc.NewServer(
			grpc.UnaryInterceptor(interceptors.GRPCServerUnaryErrorInterceptor),
			grpc.StreamInterceptor(interceptors.GRPCServerStreamErrorInterceptor),
		),
		UnimplementedAuthServiceServer: &proto.UnimplementedAuthServiceServer{},
	}
	proto.RegisterAuthServiceServer(m.grpc, m)
	return m
}

func (m *mockServer) Stop() {
	m.grpc.Stop()
}

func (m *mockServer) Addr() string {
	return m.addr
}

type ConfigOpt func(*Config)

func WithConfig(cfg Config) ConfigOpt {
	return func(config *Config) {
		*config = cfg
	}
}

func (m *mockServer) NewClient(ctx context.Context, opts ...ConfigOpt) (*Client, error) {
	cfg := Config{
		Addrs: []string{m.addr},
		Credentials: []Credentials{
			&mockInsecureTLSCredentials{}, // TODO(Joerger) replace insecure credentials
		},
		DialOpts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO(Joerger) remove insecure dial option
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return New(ctx, cfg)
}

// startMockServer starts a new mock server. Parallel tests cannot use the same addr.
func startMockServer(t *testing.T) *mockServer {
	l, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	return startMockServerWithListener(t, l)
}

// startMockServerWithListener starts a new mock server with the provided listener
func startMockServerWithListener(t *testing.T, l net.Listener) *mockServer {
	srv := newMockServer(l.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.grpc.Serve(l)
	}()

	t.Cleanup(func() {
		srv.grpc.Stop()
		require.NoError(t, <-errCh)
	})

	return srv
}

func (m *mockServer) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

func (m *mockServer) ListResources(ctx context.Context, req *proto.ListResourcesRequest) (*proto.ListResourcesResponse, error) {
	resources, err := testResources[types.ResourceWithLabels](req.ResourceType, req.Namespace)
	if err != nil {
		return nil, trail.ToGRPC(err)
	}

	resp := &proto.ListResourcesResponse{
		Resources:  make([]*proto.PaginatedResource, 0, len(resources)),
		TotalCount: int32(len(resources)),
	}

	var (
		takeResources    = req.StartKey == ""
		lastResourceName string
	)
	for _, resource := range resources {
		if resource.GetName() == req.StartKey {
			takeResources = true
			continue
		}

		if !takeResources {
			continue
		}

		var protoResource *proto.PaginatedResource
		switch req.ResourceType {
		case types.KindDatabaseServer:
			database, ok := resource.(*types.DatabaseServerV3)
			if !ok {
				return nil, trace.Errorf("database server has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_DatabaseServer{DatabaseServer: database}}
		case types.KindAppServer:
			app, ok := resource.(*types.AppServerV3)
			if !ok {
				return nil, trace.Errorf("application server has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_AppServer{AppServer: app}}
		case types.KindNode:
			srv, ok := resource.(*types.ServerV2)
			if !ok {
				return nil, trace.Errorf("node has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_Node{Node: srv}}
		case types.KindKubeServer:
			srv, ok := resource.(*types.KubernetesServerV3)
			if !ok {
				return nil, trace.Errorf("kubernetes server has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_KubernetesServer{KubernetesServer: srv}}
		case types.KindWindowsDesktop:
			desktop, ok := resource.(*types.WindowsDesktopV3)
			if !ok {
				return nil, trace.Errorf("windows desktop has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_WindowsDesktop{WindowsDesktop: desktop}}
		case types.KindAppOrSAMLIdPServiceProvider:
			appServerOrSP, ok := resource.(*types.AppServerOrSAMLIdPServiceProviderV1)
			if !ok {
				return nil, trace.Errorf("AppServerOrSAMLIdPServiceProvider has invalid type %T", resource)
			}

			protoResource = &proto.PaginatedResource{Resource: &proto.PaginatedResource_AppServerOrSAMLIdPServiceProvider{AppServerOrSAMLIdPServiceProvider: appServerOrSP}}
		}
		resp.Resources = append(resp.Resources, protoResource)
		lastResourceName = resource.GetName()
		if len(resp.Resources) == int(req.Limit) {
			break
		}
	}

	if len(resp.Resources) != len(resources) {
		resp.NextKey = lastResourceName
	}

	return resp, nil
}

func (m *mockServer) AddMFADeviceSync(ctx context.Context, req *proto.AddMFADeviceSyncRequest) (*proto.AddMFADeviceSyncResponse, error) {
	return nil, status.Error(codes.AlreadyExists, "Already Exists")
}

const fiveMBNode = "fiveMBNode"

func testResources[T types.ResourceWithLabels](resourceType, namespace string) ([]T, error) {
	size := 50
	// Artificially make each node ~ 100KB to force
	// ListResources to fail with chunks of >= 40.
	labelSize := 100000
	resources := make([]T, 0, size)

	switch resourceType {
	case types.KindDatabaseServer:
		for i := 0; i < size; i++ {
			resource, err := types.NewDatabaseServerV3(types.Metadata{
				Name: fmt.Sprintf("db-%d", i),
				Labels: map[string]string{
					"label": string(make([]byte, labelSize)),
				},
			}, types.DatabaseServerSpecV3{
				Hostname: "localhost",
				HostID:   fmt.Sprintf("host-%d", i),
				Database: &types.DatabaseV3{
					Metadata: types.Metadata{
						Name: fmt.Sprintf("db-%d", i),
					},
					Spec: types.DatabaseSpecV3{
						Protocol: types.DatabaseProtocolPostgreSQL,
						URI:      "localhost",
					},
				},
			})
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resources = append(resources, any(resource).(T))
		}
	case types.KindAppServer:
		for i := 0; i < size; i++ {
			app, err := types.NewAppV3(types.Metadata{
				Name: fmt.Sprintf("app-%d", i),
			}, types.AppSpecV3{
				URI: "localhost",
			})
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resource, err := types.NewAppServerV3(types.Metadata{
				Name: fmt.Sprintf("app-%d", i),
				Labels: map[string]string{
					"label": string(make([]byte, labelSize)),
				},
			}, types.AppServerSpecV3{
				HostID: fmt.Sprintf("host-%d", i),
				App:    app,
			})
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resources = append(resources, any(resource).(T))
		}
	case types.KindNode:
		for i := 0; i < size; i++ {
			nodeLabelSize := labelSize
			if namespace == fiveMBNode && i == 0 {
				// Artificially make a node ~ 5MB to force
				// ListNodes to fail regardless of chunk size.
				nodeLabelSize = 5000000
			}

			var err error
			resource, err := types.NewServerWithLabels(fmt.Sprintf("node-%d", i), types.KindNode, types.ServerSpecV2{},
				map[string]string{
					"label": string(make([]byte, nodeLabelSize)),
				},
			)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resources = append(resources, any(resource).(T))
		}
	case types.KindKubeServer:
		for i := 0; i < size; i++ {
			var err error
			name := fmt.Sprintf("kube-service-%d", i)
			kube, err := types.NewKubernetesClusterV3(types.Metadata{
				Name:   name,
				Labels: map[string]string{"name": name},
			},
				types.KubernetesClusterSpecV3{},
			)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			resource, err := types.NewKubernetesServerV3(
				types.Metadata{
					Name: name,
					Labels: map[string]string{
						"label": string(make([]byte, labelSize)),
					},
				},
				types.KubernetesServerSpecV3{
					HostID:  fmt.Sprintf("host-%d", i),
					Cluster: kube,
				},
			)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resources = append(resources, any(resource).(T))
		}
	case types.KindWindowsDesktop:
		for i := 0; i < size; i++ {
			var err error
			name := fmt.Sprintf("windows-desktop-%d", i)
			resource, err := types.NewWindowsDesktopV3(
				name,
				map[string]string{"label": string(make([]byte, labelSize))},
				types.WindowsDesktopSpecV3{
					Addr:   "_",
					HostID: "_",
				})
			if err != nil {
				return nil, trace.Wrap(err)
			}

			resources = append(resources, any(resource).(T))
		}
	case types.KindAppOrSAMLIdPServiceProvider:
		for i := 0; i < size; i++ {
			// Alternate between adding Apps and SAMLIdPServiceProviders. If `i` is even, add an app.
			if i%2 == 0 {
				app, err := types.NewAppV3(types.Metadata{
					Name: fmt.Sprintf("app-%d", i),
				}, types.AppSpecV3{
					URI: "localhost",
				})
				if err != nil {
					return nil, trace.Wrap(err)
				}

				appServer, err := types.NewAppServerV3(types.Metadata{
					Name: fmt.Sprintf("app-%d", i),
					Labels: map[string]string{
						"label": string(make([]byte, labelSize)),
					},
				}, types.AppServerSpecV3{
					HostID: fmt.Sprintf("host-%d", i),
					App:    app,
				})
				if err != nil {
					return nil, trace.Wrap(err)
				}

				resource := &types.AppServerOrSAMLIdPServiceProviderV1{
					Resource: &types.AppServerOrSAMLIdPServiceProviderV1_AppServer{
						AppServer: appServer,
					},
				}

				resources = append(resources, any(resource).(T))
			} else {
				sp := &types.SAMLIdPServiceProviderV1{ResourceHeader: types.ResourceHeader{Metadata: types.Metadata{Name: fmt.Sprintf("saml-app-%d", i), Labels: map[string]string{
					"label": string(make([]byte, labelSize)),
				}}}}

				resource := &types.AppServerOrSAMLIdPServiceProviderV1{
					Resource: &types.AppServerOrSAMLIdPServiceProviderV1_SAMLIdPServiceProvider{
						SAMLIdPServiceProvider: sp,
					},
				}
				resources = append(resources, any(resource).(T))
			}
		}
	default:
		return nil, trace.Errorf("unsupported resource type %s", resourceType)
	}

	return resources, nil
}

// mockInsecureCredentials mocks insecure Client credentials.
// it returns a nil tlsConfig which allows the client to run in insecure mode.
// TODO(Joerger) replace insecure credentials with proper TLS credentials.
type mockInsecureTLSCredentials struct{}

func (mc *mockInsecureTLSCredentials) Dialer(cfg Config) (ContextDialer, error) {
	return nil, trace.NotImplemented("no dialer")
}

func (mc *mockInsecureTLSCredentials) TLSConfig() (*tls.Config, error) {
	return nil, nil
}

func (mc *mockInsecureTLSCredentials) SSHClientConfig() (*ssh.ClientConfig, error) {
	return nil, trace.NotImplemented("no ssh config")
}
